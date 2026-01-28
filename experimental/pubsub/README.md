# Pub/Sub API

Pub/Sub API provides a single interface for publishing and subscribing to platform events, including real-time event monitoring events, and change data capture events. Based on [gRPC API](https://grpc.io/docs/) and HTTP/2, Pub/Sub API efficiently publishes and delivers binary event messages in the Apache Avro format.

https://developer.salesforce.com/docs/platform/pub-sub-api/guide/intro.html

Using Pub/Sub API, you can interface with the expanded and improved Salesforce event bus by publishing and subscribing to events. The event bus is a multitenant, multicloud event storage and delivery service based on a publish-subscribe model. The event bus is based on a time-ordered event log, which ensures that event messages are stored and delivered in the order that they’re received by Salesforce. Platform events and change data capture events are published to the event bus, where they’re stored for 72 hours. You can retrieve stored event messages from the event bus with Pub/Sub API. Each event message contains the Replay ID field, represented as replay_id in the protocol specification. It’s an opaque ID that identifies the event in the stream and enables replaying the stream after a specific replay ID.

**Get Started**:
1. Go to **Setup** -> **Integrations** -> **Platform Events**
2. New Platform Event

**Pub/Sub using three ways**:
1. Flows
2. Apex Triggers
3. Pub/Sub API

## References
- https://www.youtube.com/watch?v=rW7INgUU7TM 
- https://developer.salesforce.com/blogs/2024/08/build-controllable-and-scalable-integrations-with-pub-sub-api