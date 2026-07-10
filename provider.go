package vercel

import (
	"context"
	"strings"

	"github.com/libdns/libdns"
)

// Provider implements the libdns interfaces for Vercel
type Provider struct {
	// AuthAPIToken is the Vercel Authentication Token - see https://vercel.com/docs/api#api-basics/authentication
	AuthAPIToken string `json:"auth_api_token"`
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	records, err := getAllRecords(ctx, p.AuthAPIToken, unFQDN(zone))
	if err != nil {
		return nil, err
	}

	return records, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
// It never changes existing records.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var appendedRecords []libdns.Record

	for _, record := range records {
		newRecord, err := createRecord(ctx, p.AuthAPIToken, unFQDN(zone), record)
		if err != nil {
			return nil, err
		}
		appendedRecords = append(appendedRecords, newRecord)
	}

	return appendedRecords, nil
}

// DeleteRecords deletes the given records from the zone if they exist and
// exactly match the input (an empty type, TTL, or value on an input record
// matches any value for that field). It returns only the records that were
// actually deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	return deleteRecords(ctx, p.AuthAPIToken, unFQDN(zone), records)
}

// SetRecords updates the zone so that, for each (name, type) pair in the
// input, the only records in the zone with that pair are the ones provided.
// It deletes any existing records sharing a (name, type) with the input, then
// appends the input records; see [libdns.RecordSetter] for the exact
// semantics this follows.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	prevs, err := p.GetRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	var toDelete []libdns.Record
	for _, prev := range prevs {
		for _, rec := range records {
			if prev.RR().Name == rec.RR().Name && prev.RR().Type == rec.RR().Type {
				toDelete = append(toDelete, prev)
				break
			}
		}
	}

	if len(toDelete) > 0 {
		if _, err := p.DeleteRecords(ctx, zone, toDelete); err != nil {
			return nil, err
		}
	}

	return p.AppendRecords(ctx, zone, records)
}

// unFQDN trims any trailing "." from fqdn. Vercel's API does not use FQDNs.
func unFQDN(fqdn string) string {
	return strings.TrimSuffix(fqdn, ".")
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
