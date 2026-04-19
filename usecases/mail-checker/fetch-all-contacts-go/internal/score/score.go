package score

const (
	SyntaxWeight  = 25
	DomainWeight  = 25
	MXWeight      = 25
	SMTPWeight    = 25
	HistoryWeight = 0
)

type Breakdown struct {
	Syntax  int `json:"syntax"`
	Domain  int `json:"domain"`
	MX      int `json:"mx"`
	SMTP    int `json:"smtp"`
	History int `json:"history"`
	Total   int `json:"total"`
}

func FromStatus(syntaxOK, domainOK, mxOK, smtpOK bool) Breakdown {
	out := Breakdown{}
	if syntaxOK {
		out.Syntax = SyntaxWeight
	}
	if domainOK {
		out.Domain = DomainWeight
	}
	if mxOK {
		out.MX = MXWeight
	}
	if smtpOK {
		out.SMTP = SMTPWeight
	}
	out.Total = out.Syntax + out.Domain + out.MX + out.SMTP + out.History
	return out
}
