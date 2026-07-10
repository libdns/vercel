package vercel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

type getAllRecordsResponse struct {
	Records []VercelRecord `json:"records"`
}

// VercelRecord is Vercel's own wire format for a DNS record. Unlike
// [libdns.RR], it carries the provider-assigned ID needed to update or
// delete a record.
type VercelRecord struct {
	Id    string `json:"id,omitempty"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

func doRequest(token string, request *http.Request) ([]byte, error) {
	// Bearer Token for Vercel Authorization: Bearer <TOKEN>
	request.Header.Add("Authorization", "Bearer "+token)
	request.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return data, fmt.Errorf("%s (%d)", http.StatusText(response.StatusCode), response.StatusCode)
	}

	return data, nil
}

// getAllRawRecords fetches the zone's records in Vercel's own format, which
// retains the record ID that [libdns.RR] no longer carries.
func getAllRawRecords(ctx context.Context, token string, zone string) ([]VercelRecord, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.vercel.com/v4/domains/%s/records", zone), nil)
	if err != nil {
		return nil, err
	}

	data, err := doRequest(token, req)
	if err != nil {
		return nil, err
	}

	result := getAllRecordsResponse{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result.Records, nil
}

func getAllRecords(ctx context.Context, token string, zone string) ([]libdns.Record, error) {
	raw, err := getAllRawRecords(ctx, token, zone)
	if err != nil {
		return nil, err
	}

	records := make([]libdns.Record, 0, len(raw))
	for _, r := range raw {
		rec, err := vercelToRR(r).Parse()
		if err != nil {
			return nil, fmt.Errorf("parsing Vercel DNS record %+v: %v", r, err)
		}
		records = append(records, rec)
	}

	return records, nil
}

func vercelToRR(r VercelRecord) libdns.RR {
	return libdns.RR{
		Name: r.Name,
		Type: r.Type,
		Data: r.Value,
		TTL:  time.Duration(r.TTL) * time.Second,
	}
}

func createRecord(ctx context.Context, token string, zone string, record libdns.Record) (libdns.Record, error) {
	rr := record.RR()
	name := normalizeRecordName(rr.Name, zone)

	reqData := VercelRecord{
		Type:  rr.Type,
		Name:  name,
		Value: rr.Data,
		TTL:   int(math.Max(rr.TTL.Seconds(), 60)),
	}

	reqBuffer, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://api.vercel.com/v2/domains/%s/records", zone), bytes.NewBuffer(reqBuffer))
	if err != nil {
		return nil, err
	}

	if _, err := doRequest(token, req); err != nil {
		return nil, err
	}

	return (libdns.RR{
		Name: name,
		Type: rr.Type,
		Data: rr.Data,
		TTL:  time.Duration(reqData.TTL) * time.Second,
	}).Parse()
}

func deleteRecordByID(ctx context.Context, token string, zone string, id string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("https://api.vercel.com/v2/domains/%s/records/%s", zone, id), nil)
	if err != nil {
		return err
	}

	_, err = doRequest(token, req)
	return err
}

// deleteRecords implements the [libdns.RecordDeleter] contract: it only
// deletes records that exactly match an input record's name, type, TTL, and
// value, except that an empty type, TTL, or value on the input acts as a
// wildcard for that field (name is always required).
func deleteRecords(ctx context.Context, token string, zone string, records []libdns.Record) ([]libdns.Record, error) {
	raw, err := getAllRawRecords(ctx, token, zone)
	if err != nil {
		return nil, err
	}

	var deleted []libdns.Record
	for _, record := range records {
		rr := record.RR()
		name := normalizeRecordName(rr.Name, zone)

		for _, r := range raw {
			if r.Name != name {
				continue
			}
			if rr.Type != "" && r.Type != rr.Type {
				continue
			}
			if rr.Data != "" && r.Value != rr.Data {
				continue
			}
			if rr.TTL != 0 && time.Duration(r.TTL)*time.Second != rr.TTL {
				continue
			}

			if err := deleteRecordByID(ctx, token, zone, r.Id); err != nil {
				return deleted, err
			}

			parsed, err := vercelToRR(r).Parse()
			if err != nil {
				return deleted, fmt.Errorf("parsing Vercel DNS record %+v: %v", r, err)
			}
			deleted = append(deleted, parsed)
		}
	}

	return deleted, nil
}

func normalizeRecordName(recordName string, zone string) string {
	// Workaround for https://github.com/caddy-dns/hetzner/issues/3
	// Can be removed after https://github.com/libdns/libdns/issues/12
	normalized := unFQDN(recordName)
	normalized = strings.TrimSuffix(normalized, unFQDN(zone))
	return unFQDN(normalized)
}
