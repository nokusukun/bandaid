package bandaid

import (
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type DNSConfig struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int64  `json:"ttl"`
	Priority int64  `json:"priority"`
	Proxied  bool   `json:"proxied"`
}

type CloudflareConfig struct {
	Token string
	Zone  string
	DNS   DNSConfig

	apiURL  string
	devMode bool
}

func AutoCloudflare(token string) *CloudflareConfig {
	return &CloudflareConfig{
		Token: token,
		DNS: DNSConfig{
			Type:     "A",
			Name:     "",
			Content:  "",
			TTL:      1,
			Priority: 0,
			Proxied:  false,
		},
		apiURL: "https://api.cloudflare.com/client/v4"}
}

func (c *CloudflareConfig) SetZone(zone string) *CloudflareConfig {
	c.Zone = zone
	return c
}

func (c *CloudflareConfig) SetDomain(domain string) *CloudflareConfig {
	c.DNS.Name = domain
	return c
}

func (c *CloudflareConfig) SetIP(ip string) *CloudflareConfig {
	c.DNS.Content = ip
	return c
}

func (c *CloudflareConfig) Proxied(value bool) *CloudflareConfig {
	c.DNS.Proxied = value
	return c
}

func (c *CloudflareConfig) DevMode() *CloudflareConfig {
	c.devMode = true
	return c
}

func (c *CloudflareConfig) Uninstall() error {
	b, err := ioutil.ReadFile(".cf-dns")
	if err != nil {
		return err
	}
	record := DNSRecord{}
	if err = json.Unmarshal(b, &record); err != nil {
		return err
	}

	err = c.RemoveConfiguration(record)
	if err != nil {
		return err
	}

	return os.Remove(".cf-dns")
}

func (c *CloudflareConfig) Reinstall() error {
	log.Println("[cloudflare] attempting to clear installed configuration")
	err := c.Uninstall()
	if err != nil {
		log.Println("[cloudflare]  -> failed (but that's probably normal)")
	}
	return c.Install()
}

func (c *CloudflareConfig) Install() error {
	if c.devMode {
		log.Println("[cloudflare] developer flag turned on, skipping...")
		return nil
	}

	if _, err := ioutil.ReadFile(".cf-dns"); err == nil {
		log.Println("[cloudflare] '.cf-dns' record found. Skipping install...")
		return nil
	}

	record, err := c.SendConfiguration()
	if err != nil {
		return err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(".cf-dns", data, os.ModePerm)
}

func (c *CloudflareConfig) SendConfiguration() (DNSRecord, error) {
	zone, err := c.getZone()
	if err != nil {
		return DNSRecord{}, err
	}
	log.Println("[cloudflare] Zone found, installing to", zone.Name, zone.ID)

	if c.DNS.Content == "" {
		log.Print("[cloudflare] DNS.Content is empty, trying to retrieve IP address...")
		ip, err := GetIP()
		if err != nil {
			return DNSRecord{}, err
		}
		log.Println("   ->", ip)
		c.DNS.Content = ip
	}

	resp, err := grequests.Post(fmt.Sprintf("%v/zones/%v/dns_records", c.apiURL, zone.ID), &grequests.RequestOptions{
		JSON: c.DNS,
		Headers: map[string]string{
			"authorization": fmt.Sprintf("Bearer %v", c.Token),
		},
	})
	if err != nil {
		return DNSRecord{}, err
	}
	if !resp.Ok {
		return DNSRecord{}, fmt.Errorf("failed: %v", resp.String())
	}
	response, err := UnmarshalDNSRecordResponse(resp.Bytes())
	if err != nil {
		return DNSRecord{}, err
	}

	if len(response.Errors) > 0 {
		return DNSRecord{}, fmt.Errorf("%v", response.Errors)
	}
	return response.Result, nil
}

func (c *CloudflareConfig) RemoveConfiguration(record DNSRecord) error {
	req, err := grequests.Delete(fmt.Sprintf("%v/zones/%v/dns_records/%v", c.apiURL, record.ZoneID, record.ID),
		&grequests.RequestOptions{
			Headers: map[string]string{
				"authorization": fmt.Sprintf("Bearer %v", c.Token),
			},
		},
	)

	if err != nil {
		return err
	}
	if !strings.Contains(req.String(), record.ID) {
		return fmt.Errorf("unsuccessful request: %v", req.String())
	}
	return nil
}

func (c *CloudflareConfig) getZone() (*Zone, error) {
	log.Println("[cloudflare] Retrieving zone record for", c.Zone)
	resp, err := grequests.Get(fmt.Sprintf("%v/zones?name=%v", c.apiURL, c.Zone), &grequests.RequestOptions{
		Headers: map[string]string{
			"authorization": fmt.Sprintf("Bearer %v", c.Token),
		},
	})
	if err != nil {
		return nil, err
	}

	zoneResponse, err := UnmarshalZoneResponse(resp.Bytes())
	if err != nil {
		return nil, err
	}
	if len(zoneResponse.Result) == 0 {
		return nil, fmt.Errorf("no zone records found for: %v", c.Zone)
	}

	return &zoneResponse.Result[0], nil
}

func UnmarshalDNSRecordResponse(data []byte) (DNSRecordResponse, error) {
	var r DNSRecordResponse
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *DNSRecordResponse) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type DNSRecordResponse struct {
	Success  bool          `json:"success"`
	Errors   []interface{} `json:"errors"`
	Messages []interface{} `json:"messages"`
	Result   DNSRecord     `json:"result"`
}

type DNSRecord struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Name       string  `json:"name"`
	Content    string  `json:"content"`
	Proxiable  bool    `json:"proxiable"`
	Proxied    bool    `json:"proxied"`
	TTL        int64   `json:"ttl"`
	Locked     bool    `json:"locked"`
	ZoneID     string  `json:"zone_id"`
	ZoneName   string  `json:"zone_name"`
	CreatedOn  string  `json:"created_on"`
	ModifiedOn string  `json:"modified_on"`
	Data       Data    `json:"data"`
	Meta       DNSMeta `json:"meta"`
}

type Data struct {
}

type DNSMeta struct {
	AutoAdded bool   `json:"auto_added"`
	Source    string `json:"source"`
}

func UnmarshalZoneResponse(data []byte) (ZoneResponse, error) {
	var r ZoneResponse
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *ZoneResponse) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type ZoneResponse struct {
	Result     []Zone        `json:"result"`
	ResultInfo ResultInfo    `json:"result_info"`
	Success    bool          `json:"success"`
	Errors     []interface{} `json:"errors"`
	Messages   []interface{} `json:"messages"`
}

type Zone struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Status              string      `json:"status"`
	Paused              bool        `json:"paused"`
	Type                string      `json:"type"`
	DevelopmentMode     int64       `json:"development_mode"`
	NameServers         []string    `json:"name_servers"`
	OriginalNameServers []string    `json:"original_name_servers"`
	OriginalRegistrar   interface{} `json:"original_registrar"`
	OriginalDnshost     interface{} `json:"original_dnshost"`
	ModifiedOn          string      `json:"modified_on"`
	CreatedOn           string      `json:"created_on"`
	ActivatedOn         string      `json:"activated_on"`
	Meta                Meta        `json:"meta"`
	Owner               Owner       `json:"owner"`
	Account             Account     `json:"account"`
	Permissions         []string    `json:"permissions"`
	Plan                Plan        `json:"plan"`
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Meta struct {
	Step                    int64 `json:"step"`
	WildcardProxiable       bool  `json:"wildcard_proxiable"`
	CustomCertificateQuota  int64 `json:"custom_certificate_quota"`
	PageRuleQuota           int64 `json:"page_rule_quota"`
	PhishingDetected        bool  `json:"phishing_detected"`
	MultipleRailgunsAllowed bool  `json:"multiple_railguns_allowed"`
}

type Owner struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Email string `json:"email"`
}

type Plan struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Price             int64  `json:"price"`
	Currency          string `json:"currency"`
	Frequency         string `json:"frequency"`
	IsSubscribed      bool   `json:"is_subscribed"`
	CanSubscribe      bool   `json:"can_subscribe"`
	LegacyID          string `json:"legacy_id"`
	LegacyDiscount    bool   `json:"legacy_discount"`
	ExternallyManaged bool   `json:"externally_managed"`
}

type ResultInfo struct {
	Page       int64 `json:"page"`
	PerPage    int64 `json:"per_page"`
	TotalPages int64 `json:"total_pages"`
	Count      int64 `json:"count"`
	TotalCount int64 `json:"total_count"`
}
