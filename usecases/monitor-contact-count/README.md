# Monitor contacts count 

**Problems**: Salesforce pricing (especially Marketing Cloud Contacts and Data Cloud Profiles) is based on number of contacts.

**Goals**: Control cost

#### **Background**:
```bash
Loyalty Program Member Object (Created/Updated) -> API Event -> Data Extensions -> Journey -> Blast 
```

To monitor contact count and control costs in Marketing Cloud, here are the best practices:

1. Get contacts count report `/contacts/v1/addresses/count/?$pageSize=25&$page=1&$orderBy=contactKey%20ASC` API to get the current contact count. Automate this process using **Automation Studio** to regularly check the count and trigger alerts when nearing the threshold.

    > Ensure enable `audiences_read` scope first

2. Configure Automation Studio to send email notifications or alerts when the contact count approaches the limit. - This helps in taking proactive actions to avoid overages.
3. If the contact count exceeds the limit, stop active journeys to prevent further contact usage. - Use the REST API endpoint `/interaction/v1/interactions/{id}/stop` to stop a journey programmatically. Reference: https://developer.salesforce.com/docs/marketing/marketing-cloud/references/mc_rest_interaction/postStopInteractionById.html

    To implement this, we can use [Script Activity](https://help.salesforce.com/s/articleView?id=mktg.mc_as_create_a_script_activity.htm&type=5) to get contacts through SSJS.

    Or, use [SQL Query Activity](https://help.salesforce.com/s/articleView?id=mktg.mc_as_query_find_subscriber_status_ref.htm&type=5) to retrieve contacts count

4. Pause Journeys, temporarily pause journeys to halt processing contacts and sending messages. - This can be done manually in Journey Builder or programmatically using APIs Reference: https://help.salesforce.com/s/articleView?id=mktg.mc_jb_stop_journey.htm&type=5&release=260.0.0 1 
5. Manage Contact Deletion, Regularly review and delete unused or outdated contacts to free up space. - Use Contact Builder to manage and delete contacts efficiently.


### Additional
- Add filter contacts in api event. If contacts meet filter and entry mode criteria, the journey admits the contacts in response to an API request.


## Notes
- Sync Data Extensions: Records from the Lead, Contact, and User objects. Note: For Person Accounts, they are registered as Contacts based on the underlying Contact object record. If you synchronize these objects from Salesforce CRM via Sync DE, they are automatically registered as Marketing Cloud Contacts. Ref: https://help.salesforce.com/s/articleView?id=005166927&type=1