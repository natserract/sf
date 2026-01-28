package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	sfmcn "github.com/natserract/sf/pkg/salesforce/mcn"
)

func joinRecords(c *sfmcn.Salesforce) {
	sql := `SELECT ach.AccountNumber__c, ach.Name__c from Account_Home__dll AS ach INNER JOIN ssot__AccountContact__dlm AS acc ON ach.Id__c = acc.ssot__AccountId__c`

	req, err := c.PrepareRequest(
		context.Background(),
		http.MethodPost,
		"/services/data/v65.0/ssot/query-sql",
		map[string]string{
			"Content-Type": "application/json",
		},
		nil,
		map[string]string{
			"sql": sql,
		},
	)
	if err != nil {
		panic(err)
	}

	resp, err := c.CallAPI(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		panic(fmt.Errorf("query-sql failed: status=%d body=%s", resp.StatusCode, string(b)))
	}

	fmt.Println(string(b))
}
