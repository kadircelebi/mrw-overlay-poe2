package trade

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type exchangeTransport struct{ body map[string]any }

func (t *exchangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	_ = json.NewDecoder(req.Body).Decode(&t.body)
	reply := `{"id":"abc","total":3,"result":{
	  "a":{"listing":{"indexed":"2026-10-02T22:38:30+00:00","account":{"name":"Div"},"offers":[{"exchange":{"currency":"divine","amount":1},"item":{"currency":"an-audience-with-the-king","amount":100,"stock":300}}]}},
	  "b":{"listing":{"indexed":"2026-10-02T22:38:30+00:00","account":{"name":"Ex"},"offers":[{"exchange":{"currency":"exalted","amount":3},"item":{"currency":"an-audience-with-the-king","amount":2,"stock":14}}]}},
	  "c":{"listing":{"indexed":"2026-10-02T22:38:30+00:00","account":{"name":"Odd"},"offers":[{"exchange":{"currency":"annul","amount":1},"item":{"currency":"an-audience-with-the-king","amount":1,"stock":1}}]}}}}`
	if !strings.HasSuffix(req.URL.Path, "/api/trade2/exchange/poe2/Rise of the Abyssal") {
		return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(reply)), Request: req}, nil
}

// Offers are priced per item in exalted, cheapest first, unknown currencies last.
func TestExchangeOffersArePricedAndSorted(t *testing.T) {
	transport := &exchangeTransport{}
	c := NewClient("Standard", 1)
	c.http = &http.Client{Transport: transport}
	rate := func(id string) float64 { return map[string]float64{"exalted": 1, "divine": 400}[id] }

	res, err := c.Exchange(context.Background(), "Rise of the Abyssal", "an-audience-with-the-king", []string{"exalted", "divine"}, "online", rate)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Offers) != 3 || res.Total != 3 {
		t.Fatalf("offers = %+v", res.Offers)
	}
	if res.Offers[0].Account != "Ex" || res.Offers[0].ValueEx != 1.5 {
		t.Errorf("first = %+v, want Ex at 1.5 ex", res.Offers[0])
	}
	if res.Offers[1].Account != "Div" || res.Offers[1].ValueEx != 4 {
		t.Errorf("second = %+v, want Div at 4 ex", res.Offers[1])
	}
	if res.Offers[2].ValueEx != 0 {
		t.Errorf("unpriced offer should sort last: %+v", res.Offers[2])
	}
	if !strings.HasSuffix(res.TradeURL, "/trade2/exchange/poe2/Rise%20of%20the%20Abyssal/abc") {
		t.Errorf("trade url = %q", res.TradeURL)
	}
	query := transport.body["query"].(map[string]any)
	if want := query["want"].([]any); len(want) != 1 || want[0] != "an-audience-with-the-king" {
		t.Errorf("want = %v", want)
	}
}
