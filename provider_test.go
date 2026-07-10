package vercel_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"
	vercel "github.com/libdns/vercel"
)

var (
	envToken = ""
	envZone  = ""
	ttl      = time.Duration(120 * time.Second)
)

type testRecordsCleanup = func()

func toRecords(txts []libdns.TXT) []libdns.Record {
	records := make([]libdns.Record, len(txts))
	for i, t := range txts {
		records[i] = t
	}
	return records
}

func setupTestRecords(t *testing.T, p *vercel.Provider) ([]libdns.Record, testRecordsCleanup) {
	testRecords := toRecords([]libdns.TXT{
		{Name: "test1", Text: "test1", TTL: ttl},
		{Name: "test2", Text: "test2", TTL: ttl},
		{Name: "test3", Text: "test3", TTL: ttl},
	})

	records, err := p.AppendRecords(context.TODO(), envZone, testRecords)
	if err != nil {
		t.Fatal(err)
		return nil, func() {}
	}

	return records, func() {
		cleanupRecords(t, p, envZone, records)
	}
}

func cleanupRecords(t *testing.T, p *vercel.Provider, zone string, r []libdns.Record) {
	_, err := p.DeleteRecords(context.TODO(), zone, r)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}

func TestMain(m *testing.M) {
	envToken = os.Getenv("LIBDNS_VERCEL_TEST_TOKEN")
	envZone = os.Getenv("LIBDNS_VERCEL_TEST_ZONE")

	if len(envToken) == 0 || len(envZone) == 0 {
		fmt.Println(`Please notice that this test runs agains the public Vercel DNS Api, so you sould
never run the test with a zone, used in production.
To run this test, you have to specify 'LIBDNS_VERCEL_TEST_TOKEN' and 'LIBDNS_VERCEL_TEST_ZONE'.
Example: "LIBDNS_VERCEL_TEST_TOKEN="123" LIBDNS_VERCEL_TEST_ZONE="my-domain.com" go test ./... -v`)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func Test_AppendRecords(t *testing.T) {
	p := &vercel.Provider{
		AuthAPIToken: envToken,
	}

	testCases := []struct {
		records  []libdns.Record
		expected []libdns.Record
	}{
		{
			// multiple records
			records: toRecords([]libdns.TXT{
				{Name: "test_1", Text: "test_1", TTL: ttl},
				{Name: "test_2", Text: "test_2", TTL: ttl},
				{Name: "test_3", Text: "test_3", TTL: ttl},
			}),
			expected: toRecords([]libdns.TXT{
				{Name: "test_1", Text: "test_1", TTL: ttl},
				{Name: "test_2", Text: "test_2", TTL: ttl},
				{Name: "test_3", Text: "test_3", TTL: ttl},
			}),
		},
		{
			// relative name
			records: toRecords([]libdns.TXT{
				{Name: "123.test", Text: "123", TTL: ttl},
			}),
			expected: toRecords([]libdns.TXT{
				{Name: "123.test", Text: "123", TTL: ttl},
			}),
		},
		{
			// (fqdn) sans trailing dot
			records: toRecords([]libdns.TXT{
				{Name: fmt.Sprintf("123.test.%s", strings.TrimSuffix(envZone, ".")), Text: "test", TTL: ttl},
			}),
			expected: toRecords([]libdns.TXT{
				{Name: "123.test", Text: "test", TTL: ttl},
			}),
		},
		{
			// fqdn with trailing dot
			records: toRecords([]libdns.TXT{
				{Name: fmt.Sprintf("123.test.%s.", strings.TrimSuffix(envZone, ".")), Text: "test", TTL: ttl},
			}),
			expected: toRecords([]libdns.TXT{
				{Name: "123.test", Text: "test", TTL: ttl},
			}),
		},
	}

	for _, c := range testCases {
		func() {
			result, err := p.AppendRecords(context.TODO(), envZone+".", c.records)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanupRecords(t, p, envZone, result)

			if len(result) != len(c.records) {
				t.Fatalf("len(resilt) != len(c.records) => %d != %d", len(c.records), len(result))
			}

			for k, r := range result {
				got := r.RR()
				expected := c.expected[k].RR()
				if got.Type != expected.Type {
					t.Fatalf("r.Type != c.exptected[%d].Type => %s != %s", k, got.Type, expected.Type)
				}
				if got.Name != expected.Name {
					t.Fatalf("r.Name != c.exptected[%d].Name => %s != %s", k, got.Name, expected.Name)
				}
				if got.Data != expected.Data {
					t.Fatalf("r.Data != c.exptected[%d].Data => %s != %s", k, got.Data, expected.Data)
				}
				// cant check for TTL because vercel does not return any ttl values
				// if got.TTL != expected.TTL {
				// 	t.Fatalf("r.TTL != c.exptected[%d].TTL => %s != %s", k, got.TTL, expected.TTL)
				// }
			}
		}()
	}
}

func Test_DeleteRecords(t *testing.T) {
	p := &vercel.Provider{
		AuthAPIToken: envToken,
	}

	testRecords, cleanupFunc := setupTestRecords(t, p)
	defer cleanupFunc()

	records, err := p.GetRecords(context.TODO(), envZone)
	if err != nil {
		t.Fatal(err)
	}

	if len(records) < len(testRecords) {
		t.Fatalf("len(records) < len(testRecords) => %d < %d", len(records), len(testRecords))
	}

	for _, testRecord := range testRecords {
		found := false
		for _, record := range records {
			if record.RR() == testRecord.RR() {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Record not found => %+v", testRecord.RR())
		}
	}
}

func Test_GetRecords(t *testing.T) {
	p := &vercel.Provider{
		AuthAPIToken: envToken,
	}

	testRecords, cleanupFunc := setupTestRecords(t, p)
	defer cleanupFunc()

	records, err := p.GetRecords(context.TODO(), envZone)
	if err != nil {
		t.Fatal(err)
	}

	if len(records) < len(testRecords) {
		t.Fatalf("len(records) < len(testRecords) => %d < %d", len(records), len(testRecords))
	}

	for _, testRecord := range testRecords {
		found := false
		for _, record := range records {
			if record.RR() == testRecord.RR() {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Record not found => %+v", testRecord.RR())
		}
	}
}

func Test_SetRecords(t *testing.T) {
	p := &vercel.Provider{
		AuthAPIToken: envToken,
	}

	existingRecords, _ := setupTestRecords(t, p)
	newTestRecords := toRecords([]libdns.TXT{
		{Name: "new_test1", Text: "new_test1", TTL: ttl},
		{Name: "new_test2", Text: "new_test2", TTL: ttl},
	})

	allRecords := append(existingRecords, newTestRecords...)
	allRecords[0] = libdns.TXT{
		Name: allRecords[0].RR().Name,
		Text: "new_value",
		TTL:  ttl,
	}

	records, err := p.SetRecords(context.TODO(), envZone, allRecords)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupRecords(t, p, envZone, records)

	if len(records) != len(allRecords) {
		t.Fatalf("len(records) != len(allRecords) => %d != %d", len(records), len(allRecords))
	}

	if records[0].RR().Data != "new_value" {
		t.Fatalf(`records[0].RR().Data != "new_value" => %s != "new_value"`, records[0].RR().Data)
	}
}
