package testkit

import (
	"strings"
	"testing"
)

func TestFixtureIdentityAndSQL(t *testing.T) {
	identity, err := NewFixtureIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateFixtureIdentity(identity); err != nil {
		t.Fatalf("generated identity invalid: %v", err)
	}
	schema, err := SchemaSQL(identity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, identity.Marker) || !strings.Contains(schema, "ENGINE=InnoDB") {
		t.Fatal("schema lacks ownership or transactional engine")
	}
}

func TestFixtureRejectsBroadOrInjectedDatabaseNames(t *testing.T) {
	for _, name := range []string{"mysql", "app", "dba_test_", "dba_test_abc`; DROP DATABASE app; --", "dba_test_ABCDEF123456"} {
		identity := FixtureIdentity{Database: name, Marker: "0123456789abcdef01234567"}
		if _, err := DropSQL(identity); err == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
}
