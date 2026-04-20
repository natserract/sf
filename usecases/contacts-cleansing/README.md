# Contacts Cleansing

This is a limitation of Salesforce: we cannot export all contacts into external file formats such as CSV, Excel, etc.

**Current total contacts**: ~19 million records

At the moment, Salesforce only provides two approaches:

### 1. Mobile Filtered List View
We can filter attributes provided by Salesforce based on certain conditions, for example: email must not be null, etc.

However, this approach has limitations. We cannot perform deeper validations such as:
- Contact key/email syntax validation
- Domain validation
- SMTP validation
- And other advanced checks

### 2. Automation Studio
In this approach, we create SQL queries to retrieve relevant fields for further validation. The data is then copied into a Data Extension.

**Trade-offs:**
- Consumes Data Extension storage space
- Validation is limited to syntax only
- Cannot perform domain checking, SMTP validation, MX validation, etc.

## Goals

We aim to:
- Identify which contacts are valid based on:
  - Historical engagement
  - Domain information
  - Email syntax
  - SMTP validation
- Detect duplicate contact data
- Perform data cleansing to optimize contact quota (current maximum: 22 million contacts)
- Establish a continuous audit and cleansing process

## Solution

Our solution is to build an automation (custom service) outside the Salesforce platform to retrieve all contacts. Once all contacts are obtained, validation and data cleansing will be performed externally.

To achieve these goals, the following steps are required:

1. Retrieve all contacts and persist them into a local database
- Currently stored in a single private database
2. Run validation on all contacts using 5 validation layers:
- Syntax & format check
- Domain & DNS check (e.g., gmail.com)
- MX (Mail Server) check
- SMTP validation
- Engagement history (Salesforce)
3. Review and confirm results with relevant stakeholders before data cleansing
- Results are available via a dashboard
4. If approved, perform data cleansing via API (delete contact): https://developer.salesforce.com/docs/marketing/marketing-cloud/references/mc_rest_contacts/DeleteByContactIDs.html
5. Final confirmation:
- Total remaining contacts
- Backup verification

## What's inside?
- CLI
- Realtime Dashboard
