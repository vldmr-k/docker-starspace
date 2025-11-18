# Product Recommendation Service using Facebook StarSpace

This service provides product recommendations by training a **Facebook StarSpace** model on order data. Each line in your dataset represents a single order with product SKUs, for example:

```
sku_1 sku_2 sku_3
sku_6 sku_100

```

---

## Features

- **Train the Model**  
  Send a `POST` request to the `/train` endpoint with your dataset in the request body. The model learns product co-occurrence patterns from past orders.

- **Generate Embeddings**  
  After training, embeddings for each product are automatically generated for similarity searches.

- **Search for Recommendations**  
  Using DuckDB with the VSS extension, you can query recommendations via the `/recommended` endpoint. You can pass one or multiple SKUs **separated by spaces** in the `phrase` parameter:

  ```/recommended?phrase=sku_1 sku_2&limit=10

  ```

The endpoint returns the top `limit` recommended SKUs based on the combined input products.

---

## Example Usage

### 1. Train the Model

`````bash
curl -X POST http://localhost:PORT/train -d @orders.txt

2. Get Recommendations for a Single SKU

````bash
curl http://localhost:PORT/recommended?phrase=sku_1&limit=5
`````

---

### Requirements

    • Facebook StarSpace
    • DuckDB with VSS extension
    • Python 3.8+ (or your preferred runtime)

### How It Works

    1. Prepare your dataset: One order per line, SKUs separated by spaces.
    2. Train the model: The /train endpoint reads the dataset and trains the StarSpace model.
    3. Generate embeddings: After training, embeddings for each SKU are created.
    4. Query recommendations: Use the /recommended endpoint with one or more SKUs to get related product suggestions.

#### Use Case

This repository is ideal for e-commerce platforms looking to generate SKU-level product recommendations based on historical order data, helping increase cross-sells and improve customer experience.
