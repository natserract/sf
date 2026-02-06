# API Event

## Limitation
1. < 50 records per transaksi. Marketing Cloud Next: > 5.496
2. maksimal 2.000 flow invocations. (incl: apex trigger, flow: record updated)
3. Batas waktu CPU
4. Batas 50 System.enqueueJob
    Setiap callout ke Journey Builder masuk sebagai invocable method. Dalam satu transaksi hanya bisa ada:

    Maksimal 50 enqueue job, dan

    Maksimal 200 record yang diproses.
    Jika satu objek memenuhi banyak kriteria event, batas ini bisa cepat tercapai. Batas ini juga dipakai bersama dengan trigger dan flow lain di Salesforce.

**Additional References**:
- https://help.salesforce.com/s/articleView?id=mktg.mc_overview_limits_api.htm&type=5
- https://help.salesforce.com/s/articleView?id=mktg.mc_jb_salesforce_data_event.htm&type=5
- https://developer.salesforce.com/docs/marketing/marketing-cloud/guide/how-to-fire-an-event.html

## Test Scenarios
1. Send 60 events simultaneously in one batch. Objective: Exceed 50 records per transaction limit

    > 60 events * 1 record = 60 total records (Exceed 50 records per transaction limit)

2. Send 50 events where each event creates 5 records (= 250 total records). Objective: Exceed 200 record processing limit within enqueue jobs 

    > 60 events * 5 records = 300 total records (This exceeds the 200 record limit)

3. Send 2,500 events within 10 minutes. Objective: Exceed 2,000 flow invocations

4. Send 60 events where each contact qualifies for 2 different journeys (= 120 jobs). Objective: Exceed 50 enqueue jobs per transaction

## Running
```bash
k6 run -e AUTH_API_ENDPOINT=${domain}/v2/token -e CLIENT_ID=XXX -e CLIENT_SECRET=XXX loadtest.js
```