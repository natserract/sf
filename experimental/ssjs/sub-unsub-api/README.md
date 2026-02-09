# Subscribe/Unsubscribe 

Goals: Run subscribe/unsubscribe using SSJS deployed to cloudpages (as endpoint)

## Running
Endpoint: `https://CLOUDPAGES_DOMAIN`

Payload:
```json
{
    "action": "unsubscribe",
    "subscriberKey": "alfins132@gmail.com",
    "emailAddress": "alfins132@gmail.com"
}
```

Response:
```json
{
    "success": true,
    "status": "OK",
    "message": "Subscriber globally unsubscribed successfully",
    "details": [
        {
            "Object": {
                "SubscriberKey": "alfins132@gmail.com",
                "Client": null,
                "ID": 230271120,
                "EmailAddress": "alfins132@gmail.com",
                "Attributes": null,
                "UnsubscribedDate": "0001-01-01T00:00:00.000",
                "Status": "Unsubscribed",
                "PartnerType": null,
                "EmailTypePreference": "Text",
                "Lists": null,
                "GlobalUnsubscribeCategory": null,
                "SubscriberTypeDefinition": null,
                "Addresses": null,
                "PrimarySMSAddress": null,
                "PrimarySMSPublicationStatus": "OptedIn",
                "PrimaryEmailAddress": null,
                "Locale": null,
                "PartnerKey": null,
                "PartnerProperties": null,
                "CreatedDate": "0001-01-01T00:00:00.000",
                "ModifiedDate": null,
                "ObjectID": null,
                "CustomerKey": null,
                "Owner": null,
                "CorrelationID": null,
                "ObjectState": null,
                "IsPlatformObject": false
            },
            "UpdateResults": null,
            "ParentPropertyName": null,
            "StatusCode": "OK",
            "StatusMessage": "Updated Subscriber.",
            "OrdinalID": 0,
            "ErrorCode": 0,
            "RequestID": null,
            "ConversationID": null,
            "OverallStatusCode": null,
            "RequestType": "Synchronous",
            "ResultType": null,
            "ResultDetailXML": null
        }
    ]
}
```