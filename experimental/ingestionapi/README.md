# Ingestion API

Ingestion API is a REST API and offers two interaction patterns: bulk and streaming. The streaming pattern accepts incremental updates to a dataset as those changes are captured, while the bulk pattern accepts CSV files in cases where data syncs occur periodically. The same data stream can accept data from the streaming and the bulk interaction.

## Ingestion API limitations:
- Batch only
- Maximum Payload Size: 200 KB maximum body size per request (JSON data).
- Request Rate: 250 requests per second across all ingestion API object endpoints.
- Latency: Data is processed asynchronously with an expected latency of approximately 3 minutes.
- Deletions: A maximum of 200 records can be deleted via a single streaming API request. 
- Status data yang masuk akan terdelay di Data Cloud UI

## References
- https://www.youtube.com/watch?v=3xWSVGcTORI 
- https://developer.salesforce.com/docs/data/data-cloud-int/references/data-cloud-ingestionapi-ref/c360-a-api-ingest-data.html