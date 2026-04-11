# Custom Create Contact API

Objective: Create or update a contact AND assign subscription status (Subscribed / Unsubscribed) in a single unified flow based on user consent

## Problems to Solve
We need single endpoint to handle create contact, this endpoint will triggered from [http callout](https://www.cloudsciencelabs.com/blog/http-callout-into-flow-builder-without-code) in flow builder. We have requirement during creating contact, it needs follow these steps:
- Detecting country phone number
- Create contact + consent handling

So that's why we need single endpoint to manage multiple workflows. 

## Logic Flow
```bash
[Loyalty]
-> Processing external_id with prefix
-> User/member validation (e.g. already registered?)
       ↓
Send Request (external_id, phone, email, first_name, last_name, is_whatsapp_consent, is_email_consent)
       ↓
[Custom API Layer]
       ↓
1. Detect Locale (from phone)
2. Create contact + apply consent logic
    → If consent = true → Subscribed
    → If consent = false → Unsubscribed (Contact)
       ↓
[Salesforce Marketing Cloud]
       ↓
✔ Contact created
```

## Expectation
- The endpoint should running all criterias above
- The endpoint should handle retry mechanism logic
- The endpoint should have error logs
- The endpoint shouldn't generate auth token every time (cached)

## Result
@see [create-contact.ssjs](./create-contact.ssjs)