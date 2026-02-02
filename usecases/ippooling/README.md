# Volume limitation in Salesforce

We are experiencing a sending volume limitation in Salesforce, where email delivery is capped at 2 million emails per IP per day.

Currently, we have 4 business units, and 1 business unit uses a single dedicated IP. Our requirement is to send up to 10 million emails per day from that business unit.

To achieve this, we plan to:
- Use IP Pooling to distribute sending volume evenly across multiple IPs
- Apply throttling by spreading delivery over 12–16 hours (not burst sending)


Planned setup for **1 Business Unit**:
- Existing IP: 2M
- IP-2: 2M
- IP-3: 2M
- IP-4: 2M
- IP-5: 2M


Total: **10M emails/day** (with 4 additional dedicated IPs)
We have a few questions:

1. In Salesforce, under Delivery Profile → IP Address, Salesforce recommends using the Default Profile. 
    - Does the Default Profile automatically distribute sending across all available IPs, or does it use only one IP?

2. Is it possible to configure Salesforce so that when one IP reaches the 2M daily limit, sending will automatically switch to another available IP within the same business unit?


## Solution

First, we need to know how IP Pooling Works with **Account Default**

- When you select Account Default in a single [delivery profile](https://help.salesforce.com/s/articleView?id=mktg.mc_es_delivery_profiles.htm&type=5), Salesforce Marketing Cloud automatically distributes the sending volume across all the dedicated IPs in your account's default pool. 

- This means you only need one delivery profile with **Account Default** selected, and the system will handle the distribution of emails across the available IPs in the pool.

When Account Default is selected, the sending IP address is chosen from your account's default pool for the specified send classification type. For a list of IP addresses in your account's default pool, log a ticket with Salesforce Customer Support. To send with a single dedicated IP address, select Private, and select an address from the dropdown.

### When to Create Multiple Delivery Profiles

You would only need multiple delivery profiles if:
1. You want to use specific IPs for different types of emails (e.g., transactional vs. marketing emails). 
2. You have different business units or campaigns that require separate configurations.

### Steps to Configure IP Pooling
1. Create or edit a single delivery profile. 
2. In the IP Address section, select Account Default. 
3. Save the delivery profile. 
4. Use this delivery profile for your email sends, and the system will automatically balance the sending volume across the IPs in the pool.

To enable IP Pooling with Account Default, you need to purchase multiple dedicated IPs (e.g., 4 IPs) and configure a single delivery profile with Account Default.

### Account Default
When you select Account Default in the delivery profile, the system automatically distributes the sending volume across all the dedicated IPs in your account's default pool. This ensures balanced usage of the IPs and helps maintain a good sending reputation.

----

## Salesforce Support Response (Confirmed)
When multiple Dedicated IPs are configured as Default Sending IPs, Salesforce Marketing Cloud automatically distributes the sending volume across all available IPs. The platform uses a round-robin method, typically switching IPs approximately every 5,000 emails, and the Dedicated IP selected for each batch is chosen randomly. As a result, the overall sending volume is evenly distributed across the IP pool rather than relying on a single IP, which allows you to scale sending while maintaining stability and deliverability.

We do not currently support an automatic mechanism where sending stops on one IP after it reaches the daily 2M limit and then switches exclusively to another IP. Instead, all Default Sending IPs are used concurrently throughout the send, and volume is balanced across them. Because of this, your plan to apply throttling and spread delivery evenly over 12–16 hours is strongly recommended and aligns with best practices to prevent IP saturation and protect sender reputation.

## Additional Readings
- https://help.salesforce.com/s/articleView?id=mktg.mc_es_delivery_profiles.htm&type=5