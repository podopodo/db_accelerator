package testkit

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

const FixturePrefix = "dba_test_"

var fixtureNamePattern = regexp.MustCompile(`^dba_test_[a-z0-9]{12}$`)

type FixtureIdentity struct {
	Database string
	Marker   string
}

// NewFixtureIdentity creates names that cleanup code can recognize narrowly.
func NewFixtureIdentity() (FixtureIdentity, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return FixtureIdentity{}, err
	}
	encoded := hex.EncodeToString(buffer)
	return FixtureIdentity{
		Database: FixturePrefix + encoded[:12],
		Marker:   encoded,
	}, nil
}

func ValidateFixtureIdentity(identity FixtureIdentity) error {
	if !fixtureNamePattern.MatchString(identity.Database) {
		return errors.New("fixture database name is outside reserved pattern")
	}
	if len(identity.Marker) != 24 || strings.Trim(identity.Marker, "0123456789abcdef") != "" {
		return errors.New("fixture ownership marker is invalid")
	}
	return nil
}

// SchemaSQL returns deterministic fixture DDL after strict identifier validation.
func SchemaSQL(identity FixtureIdentity) (string, error) {
	if err := ValidateFixtureIdentity(identity); err != nil {
		return "", err
	}
	return "CREATE DATABASE `" + identity.Database + "`;\n" +
		"USE `" + identity.Database + "`;\n" +
		"CREATE TABLE `_dba_fixture_owner` (`marker` CHAR(24) PRIMARY KEY, `version` INT NOT NULL) ENGINE=InnoDB;\n" +
		"INSERT INTO `_dba_fixture_owner` (`marker`, `version`) VALUES ('" + identity.Marker + "', 1);\n" +
		"CREATE TABLE `accounts` (`id` BIGINT PRIMARY KEY, `balance_cents` BIGINT NOT NULL, `version` BIGINT NOT NULL DEFAULT 0) ENGINE=InnoDB;\n" +
		"CREATE TABLE `events` (`id` BINARY(16) PRIMARY KEY, `account_id` BIGINT NOT NULL, `kind` VARCHAR(32) NOT NULL, `payload` VARBINARY(1024) NOT NULL, `created_at` TIMESTAMP(6) NOT NULL, INDEX `events_account_created` (`account_id`, `created_at`), CONSTRAINT `events_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`)) ENGINE=InnoDB;\n", nil
}

// DropSQL is intentionally separate. Callers must verify the ownership row on
// the same server before executing the returned statement.
func DropSQL(identity FixtureIdentity) (string, error) {
	if err := ValidateFixtureIdentity(identity); err != nil {
		return "", err
	}
	return "DROP DATABASE `" + identity.Database + "`", nil
}
