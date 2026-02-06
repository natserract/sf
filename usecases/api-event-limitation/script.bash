curl --location 'https://mcx3dk6gqx05byn626r3yqc9-hl0.rest.marketingcloudapis.com/interaction/v1/events' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsImtpZCI6IjQiLCJ2ZXIiOiIxIiwidHlwIjoiSldUIn0.eyJhY2Nlc3NfdG9rZW4iOiJZY2twd2RLcUFiTUkybll1NThxWEx3ODkiLCJjbGllbnRfaWQiOiJnbmE4bDFzNGxlajVobGxld3R0cWhydTkiLCJlaWQiOjExMDAwNjQ3NCwic3RhY2tfa2V5IjoiUzExIiwicGxhdGZvcm1fdmVyc2lvbiI6MiwiY2xpZW50X3R5cGUiOiJTZXJ2ZXJUb1NlcnZlciIsInBpZCI6NzI5fQ.v6JkI1LxVaontmxL-FT9hl8U19YT7sg7BmimQzxPVfQ.AYlGU0oeDS2TTYcYDgKu2pgug4XzoEk5oJCOfCxQoC5k3ZodDLLL8Mszei59ueSnC2rvIXFURGA_DLO6kJEAQ6xYtDBnkc7lYpO6vyYavOar5nDBYH-8EJk63QD_EWA5MjeCJAWzP3qcCb_emYQ5JFDMD6kgPuItm6OXL' \
--data-raw '{
  "ContactKey": "bima@blendmedia.co.id",
  "EventDefinitionKey": "APIEvent-33b86a60-9379-f8e3-599d-a559c67640a0",
  "Data": {
    "ContactKey": "bima@blendmedia.co.id",
    "Email": "bima@blendmedia.co.id",
    "FullName": "Bima Adhitya Sukoco",
    "TierName": "Crew",
    "PointsBalance": "0.00"
  }
}
'

# Journey: Loyalty_Admin154_Welcome_Marketing_Campaign
# API Event: API_Event_Welcome_Campaign
# Event Definition Key: APIEvent-33b86a60-9379-f8e3-599d-a559c67640a0
# DE Path: Data Extensions -> Loyalty DE -> DE_Welcome_Campaign
# Testing Log: Data Extensions -> Testing -> WelcomeCampaignLog

curl --location 'https://mcx3dk6gqx05byn626r3yqc9-hl0.auth.marketingcloudapis.com/v2/token' \
--header 'Content-Type: application/json' \
--data '{
    "grant_type": "client_credentials",
    "client_id": "gna8l1s4lej5hllewttqhru9",
    "client_secret": "fCrd1lqfNBu9qaDOgcnDD0c4"
}'