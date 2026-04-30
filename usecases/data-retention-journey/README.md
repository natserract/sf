# Setting data retention to 7 days

For all data extension that have a journey or sendable true

Creating go CLI application used for enable all data extension to 7 days, it needs several steps:

**Steps:**
1. Find any data extension that used in journey or `isSendable` true. To achieve that we need several steps:
    - Retrieve all journeys `1.Get-All-Journeys.bash`
    - Get detail of journey: `2.Get-Detail-Journey.bash`. Get `metaData.eventDefinitionId`
    - Get detail of event definitions: `3.Get-Event-Definition.bash`. Get `"dataExtensionId": "{{DATA_EXTENSION_ID}}"`
    - Fetch data extension detail `4.Get-Data-Extension-Detail.bash`
    - Update data retention `5.Update-Data-Retention.bash`

2. Export prev data to CSV (include name, etc) - as a backup
3. Set data retention to 7 days
4. Export updated data to CSV (separate file)

The go app must:
- performant, use concurrent to make it fast. Read and write
- in cli app, it needs some authentication credentials parameter in cli for all those bash i've shared. please check it which one dynamic value. You need to ensure all authentication verified with ping first during application start. If got 401 unauthenticated, ask the user to input auth credentials again
- durable