package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	_ "github.com/duckdb/duckdb-go/v2"
)

// Replace with your actual paths
const (
	TRAIN_DIM_PARAM = 50

	STARSPACE_PATH = "/opt/starspace"

	DATA_PATH = "/data"
)

func main() {
	e := echo.New()

	dbm := &DBManager{
		DB_PATH: DATA_PATH + "/embeddings.duckdb",
	}

	conn, _ := dbm.Open()
	dbm.Setup(conn)

	sp := StarSpace{
		DBManager: dbm,

		MODEL_PATH:     DATA_PATH + "/model",
		MODEL_TSV_PATH: DATA_PATH + "/model.tsv",

		STARSPACE_EMBED_DOC_PATH: STARSPACE_PATH + "/embed_doc",
	}

	e.GET("/recomended", func(c echo.Context) error {
		phrase := c.QueryParam("phrase")

		limitStr := c.QueryParam("limit")
		limit := 10
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}

		output, _ := sp.FindEmbeddings(phrase)
		records, err := dbm.Search(output, limit)
		if err != nil {
			fmt.Println("Error ", err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "records": records})
	})

	e.POST("/train", func(c echo.Context) error {
		// Read the body
		bodyBytes, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to read body")
		}

		// Create a temp file
		tmpFile, err := os.CreateTemp("", "body-*.txt") // pattern can be customized
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to create temp file")
		}
		defer tmpFile.Close()

		// Write body to temp file
		if _, err := tmpFile.Write(bodyBytes); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to write to temp file")
		}

		go sp.Process(tmpFile.Name())

		// Respond with temp file name
		// Respond with JSON
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Body saved successfully",
			"file":    tmpFile.Name(),
		})
	})
	e.Logger.Fatal(e.Start(":8000"))
}

type DBManager struct {
	DB_PATH string
	DB      *sql.DB

	DB_MEMORY *sql.DB
}

func (dbm *DBManager) Open() (*sql.DB, error) {
	db, err := sql.Open("duckdb", dbm.DB_PATH+"?hnsw_enable_experimental_persistence=true")
	dbm.Setup(db)
	if err != nil {
		log.Fatal(err)
	}
	dbm.DB = db
	return db, err
}

func (DBManager *DBManager) OpenMemory() *sql.DB {
	db, err := sql.Open("duckdb", ":memory:")
	DBManager.Setup(db)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func (DBManager *DBManager) SyncMemoryToPersistence(memory *sql.DB) error {
	_, err := memory.Exec(`ATTACH '/data/embeddings_2.duckdb';
			COPY FROM DATABASE memory TO embeddings;
			DETACH embeddings;
	`)

	return err
}

// func (DBManager *DBManager) syncToPersistence(memory *sql.DB) (sql.Result, error) {
// 	return memory.Exec(`.backup ` + EMBEDDINGS_SQLITE_DB_PATH)
// }

func (DBManager *DBManager) Setup(conn *sql.DB) {

	_, err := conn.Exec(`INSTALL vss; LOAD vss;`)
	if err != nil {
		fmt.Println(err.Error())
	}

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS embeddings (item VARCHAR PRIMARY KEY, embedding FLOAT[50], updated TIMESTAMP);`)
	if err != nil {
		fmt.Println(err.Error())
	}

	_, err = conn.Exec(`CREATE INDEX IF NOT EXISTS  embeddings_embedding_idx ON embeddings USING HNSW(embedding) WITH (metric = 'cosine');`)
	if err != nil {
		fmt.Println(err.Error())
	}
}

type Record struct {
	Item      string    `json:"item"`
	Emgedding []float32 `json:"-"`
}

func (DBManager *DBManager) Search(queryVec []float32, limit int) ([]Record, error) {
	var records []Record
	rows, err := DBManager.DB.Query("SELECT item FROM embeddings ORDER BY array_distance(embedding, ?::FLOAT[50]) LIMIT ?;", queryVec, limit)

	if err != nil {
		return nil, err
	}

	for rows.Next() {
		record := new(Record)
		err := rows.Scan(&record.Item)
		if err != nil {
			log.Fatal(err)
		}
		records = append(records, *record)
	}

	fmt.Println(records)

	return records, err
}

type StarSpace struct {
	DBManager *DBManager

	MODEL_PATH     string
	MODEL_TSV_PATH string

	STARSPACE_EMBED_DOC_PATH string
}

func (sp *StarSpace) Process(newDataset string) error {
	log.Println("Begin train model")
	if err := sp.Train(newDataset); err != nil {
		return fmt.Errorf("train failed: %w", err)
	}
	log.Println("End train model")

	log.Println("Begin generate embeddings")
	if err := sp.GenerateEmbeddings(); err != nil {
		return fmt.Errorf("generate embeddings failed: %w", err)
	}
	log.Println("End generate embeddings")
	return nil
}

func (sp *StarSpace) FindEmbeddings(phrase string) ([]float32, error) {
	// Build the full shell pipeline
	cmdText := fmt.Sprintf(`echo "%s" | %s %s | tail -1 | tr ' ' ','`, phrase, sp.STARSPACE_EMBED_DOC_PATH, sp.MODEL_PATH)

	cmd := exec.Command("bash", "-c", cmdText)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	result := out.String()
	if err != nil {
		return []float32{}, fmt.Errorf("error: %v\noutput: %s", err, result)
	}

	return sp.parseStrToFloatArray(result), nil

}

func (sp *StarSpace) parseStrToFloatArray(input string) []float32 {
	// Parse CSV into []float32
	parts := strings.FieldsFunc(input, func(r rune) bool { return r == ',' })
	numbers := make([]float32, 0, len(parts))
	for _, p := range parts {
		if f, err := strconv.ParseFloat(p, 32); err == nil {
			numbers = append(numbers, float32(f))
		}
	}

	return numbers
}

func (sp *StarSpace) Train(dataset string) error {
	// Build the command arguments
	args := []string{
		"train",
		"-trainFile", dataset,
		"-model", sp.MODEL_PATH,
		"-label", "''",
		"-trainMode", "5",
		"-epoch", "25",
		"-dim", strconv.Itoa(TRAIN_DIM_PARAM),
	}

	cmd := exec.Command(STARSPACE_PATH+"/starspace", args...)

	output, _ := cmd.StdoutPipe()
	cmd.Start()

	scanner := bufio.NewScanner(output)
	// Increase buffer limits
	const maxCapacity = 1024 * 1024 * 100 // 100MB
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	return cmd.Wait()
}

func (sp *StarSpace) GenerateEmbeddings() error {
	// Ensure model TSV exists
	if _, err := os.Stat(sp.MODEL_TSV_PATH); err != nil {
		return fmt.Errorf("model tsv not found: %w", err)
	}

	file, err := os.Open(sp.MODEL_TSV_PATH)
	if err != nil {
		return fmt.Errorf("failed to open model tsv: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed reading tsv: %w", err)
	}

	conn := sp.DBManager.DB

	// Begin transaction
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = tx.Exec("DELETE FROM embeddings;")
	if err != nil {
		fmt.Println("error clean db", err.Error())
	}

	// Prepare statement inside transaction
	stmt, err := tx.Prepare(`
		INSERT INTO embeddings (item, embedding, updated) VALUES (?, ?, now()) 
		ON CONFLICT DO UPDATE SET embedding = EXCLUDED.embedding, updated = now();
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		item := record[0]

		embeddings, err := sp.FindEmbeddings(item)
		if err != nil {
			log.Printf("warning: failed to find embeddings for '%s': %v", item, err)
			continue
		}

		if _, err := stmt.Exec(item, embeddings); err != nil {
			log.Printf("warning: failed to insert embeddings for '%s': %v", item, err)
			continue
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
