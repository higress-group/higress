# Bid Information Query

An MCP server for querying **Chinese tender notices and bid-award results**, hosted by Higress. Connect it to your AI assistant (Claude, Cursor, Cherry Studio, etc.) and search nationwide bidding information or analyze competitors' award history in natural language.

The dataset covers tender and award information from all provinces and cities across China, updated in real time.

## What It Can Do

- 🔍 **Search tender notices**: Filter tender notices by keyword, date range, province, and city
- 🏆 **Search award results**: Query bid-award / deal results, with filtering by winning company name
- 📄 **Get notice details**: View the full content of a single notice

## Available Tools

### 1. `search_bid_notices` — Search Tender Notices

Search tender notices by keyword, with optional date and region filters.

| Parameter      | Type   | Required | Description                                             |
| -------------- | ------ | -------- | ------------------------------------------------------- |
| `keyword`    | string | ✅       | Search keyword                                          |
| `start_date` | string |          | Start date (format `YYYY-MM-DD`)                      |
| `end_date`   | string |          | End date (format `YYYY-MM-DD`)                        |
| `province`   | string |          | Project province (full Chinese name, e.g. `广东省`)  |
| `city`       | string |          | Project city (full Chinese name, e.g. `深圳市`)      |
| `page`       | int    |          | Page number, starting from 1 (default 1)                |
| `page_size`  | int    |          | Items per page (default 20, max 100)                    |

### 2. `search_bid_results` — Search Award Results

Query bid-award / deal results. Same parameters as tender notices, plus filtering by winning company.

| Parameter                     | Type   | Required | Description                                |
| ----------------------------- | ------ | -------- | ------------------------------------------ |
| `keyword`                   | string | ✅       | Search keyword                             |
| `company_name`              | string |          | Filter by winning company name             |
| `start_date` / `end_date` | string |          | Date range (`YYYY-MM-DD`)                 |
| `province` / `city`       | string |          | Project region (full Chinese name)         |
| `page` / `page_size`      | int    |          | Pagination parameters                      |

### 3. `get_bid_detail` — Get Notice Detail

Retrieve the full content of a single notice by document ID (HTML tags stripped from the body text).

| Parameter      | Type   | Required | Description                                          |
| -------------- | ------ | -------- | ---------------------------------------------------- |
| `doc_id`     | string | ✅       | Document ID (from the `id` field of search results) |
| `index_type` | string | ✅       | `notice` (tender notice) or `result` (award result) |

## Usage Examples

Once connected, just ask your AI assistant in natural language:

**Search for tenders:**

> Search tender notices about "smart city" in Guangdong Province over the last month

> Find IT infrastructure tender projects in Beijing since 2026

**Analyze competitors:**

> Analyze Huawei Technologies' bid wins this year — which projects did they win?

> Show me which healthcare projects "Sinosoft" has won

**View details:**

> Show me the full content of the first tender notice from the previous search

**Recommended workflow:**

1. Start with `search_bid_notices` to see what projects are being tendered
2. Then use `search_bid_results` to see who is winning — understand the competitive landscape
3. For notices of interest, use `get_bid_detail` to get the complete content

## Usage Limits

To keep the service stable, the following limits apply:

- **Request rate**: Up to **50 requests per IP per day** by default; a rate-limit message is returned when exceeded
- **Pagination**: A single query returns at most 100 items and deep pagination is limited — use more precise keywords to narrow results

Contact the service administrator if you need a higher quota.

## FAQ

**Q: Connection failed — what should I do?**
A: Check: ① the URL is correct; ② the address is reachable from your network.

**Q: I'm getting rate-limited — what should I do?**
A: The default quota is 50 requests per IP per day. Reduce your request rate or contact the administrator for a higher quota.

**Q: No search results?**
A: Try: ① broader keywords; ② removing date or region filters; ③ using full Chinese names for province/city (e.g. "广东省" instead of "广东").

**Q: What date format is required?**
A: Always use `YYYY-MM-DD`, e.g. `2026-09-01`.

## Technical Support

If you encounter any issues while using this service, please contact the service administrator for support.
