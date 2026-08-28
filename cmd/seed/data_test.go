package main

import (
	"os"
	"regexp"
	"testing"
)

// cuisinesMigration holds the reference data the seeded venues link to.
const cuisinesMigration = "../../migrations/kitchen/000002_cuisines.up.sql"

var cuisineRow = regexp.MustCompile(`\('([a-z_]+)',`)

// A venue whose cuisine slug is misspelled is inserted without a single
// cuisine and never shows up under a filter: the seed reports no error.
func TestVenueCuisinesExistInTheReferenceData(t *testing.T) {
	t.Parallel()

	migration, err := os.ReadFile(cuisinesMigration)
	if err != nil {
		t.Fatalf("read cuisines migration: %v", err)
	}

	known := map[string]bool{}
	for _, match := range cuisineRow.FindAllStringSubmatch(string(migration), -1) {
		known[match[1]] = true
	}

	if len(known) == 0 {
		t.Fatalf("no cuisines found in %s", cuisinesMigration)
	}

	for _, v := range venues {
		t.Run(v.slug, func(t *testing.T) {
			t.Parallel()

			if len(v.cuisines) == 0 {
				t.Fatal("venue has no cuisines and cannot be found by filter")
			}

			for _, slug := range v.cuisines {
				if !known[slug] {
					t.Errorf("cuisine %q is not in the reference data", slug)
				}
			}
		})
	}
}

func TestVenueFixturesAreUnique(t *testing.T) {
	t.Parallel()

	cases := map[string]func(venue) string{
		"id":         func(v venue) string { return v.id },
		"slug":       func(v venue) string { return v.slug },
		"api key":    func(v venue) string { return v.apiKey },
		"venue name": func(v venue) string { return v.name },
	}

	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			seen := map[string]bool{}
			for _, v := range venues {
				value := field(v)
				if value == "" {
					t.Errorf("venue %q has an empty %s", v.slug, name)
				}
				if seen[value] {
					t.Errorf("%s %q is used by more than one venue", name, value)
				}
				seen[value] = true
			}
		})
	}
}

// Items are matched by external_id within a venue, so a duplicate would make
// the second one silently replace the first on the next menu upload.
func TestMenuExternalIDsAreUniqueWithinVenue(t *testing.T) {
	t.Parallel()

	for _, v := range venues {
		t.Run(v.slug, func(t *testing.T) {
			t.Parallel()

			categories := map[string]bool{}
			items := map[string]bool{}

			for _, c := range v.menu {
				if categories[c.externalID] {
					t.Errorf("category %q is declared twice", c.externalID)
				}
				categories[c.externalID] = true

				for _, i := range c.items {
					if items[i.externalID] {
						t.Errorf("item %q is declared twice", i.externalID)
					}
					items[i.externalID] = true

					if i.price <= 0 {
						t.Errorf("item %q has a non-positive price", i.externalID)
					}
				}
			}
		})
	}
}
