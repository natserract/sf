package api

type TypedString struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Address struct {
	ContactKey TypedString `json:"contactKey"`
	ContactID  TypedString `json:"contactID"`
}

type Response struct {
	PageNumber int       `json:"pageNumber"`
	PageSize   int       `json:"pageSize"`
	TotalCount int       `json:"totalCount"`
	Addresses  []Address `json:"addresses"`
}

type CountResponse struct {
	TotalCount int `json:"totalCount"`
}

type ContactInfo struct {
	ContactKey string
	ContactID  string
}

type MessageHistoryResponse struct {
	DataSources             []any  `json:"dataSources"`
	Messages                []any  `json:"messages"`
	RequestServiceMessageID string `json:"requestServiceMessageID"`
	ResponseDateTime        string `json:"responseDateTime"`
	ResultMessages          []any  `json:"resultMessages"`
	ServiceMessageID        string `json:"serviceMessageID"`
}
