package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	branchkit "github.com/branchkit/plugin-sdk-go"
	"github.com/branchkit/plugin-sdk-go/ui"
)

// The trait EDITOR — reading traits back with the user's edits applied, and
// writing those edits.
//
// The publishing half (buildAppTraits, pushAppTraitsCollection) lives in
// apps.go beside the app scan it shares a snapshot with. This file is the part
// that only exists because there is a screen.
//
// Editing does NOT require opening the collection to other plugins. `writers`
// stays `introducer_only`: user edits travel as platform OVERRIDES in the
// `_user` band, which `overrides.apply` permits for a collection this plugin
// introduced. That keeps the two halves of stage two separable — a person can
// retag their apps without any third party gaining a write.

// traitCatalog is one render's worth of trait state: what each app has, and
// how many apps carry each trait.
type traitCatalog struct {
	// ByBundle is the COMPOSED view — shipped traits with the user's additions
	// and removals applied, not the shipped file.
	ByBundle map[string][]string
	// Counts feeds the "terminal (7)" labels on the add menu. They are the
	// typo backstop: a stray `terminl (1)` next to `terminal (7)` gives itself
	// away where the mistake is made, with no validation rule needed.
	Counts map[string]int
	// Names is every trait that exists, sorted, for the add menu.
	Names []string
	// Loaded is false when the collection could not be read. An empty catalog
	// and a failed read are the same map and different facts — the same
	// distinction the voice plugin needs on its side.
	Loaded bool
}

// loadTraitCatalog reads the composed collection.
//
// Deliberately re-read per render rather than cached: this is a settings page,
// the collection holds tens of records, and a cache here would need
// invalidating on every edit the page itself makes. The publishing path caches;
// the editor does not need to.
func loadTraitCatalog(p *branchkit.Plugin) traitCatalog {
	cat := traitCatalog{ByBundle: map[string][]string{}, Counts: map[string]int{}}
	if p == nil {
		return cat
	}
	records, err := p.ListAll(appTraitsCollection)
	if err != nil {
		branchkit.Logf("system", "trait catalog read error: %v", err)
		return cat
	}
	for _, rec := range records {
		var payload struct {
			Trait    string `json:"trait"`
			BundleID string `json:"bundle_id"`
		}
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			continue
		}
		if payload.Trait == "" || payload.BundleID == "" {
			continue
		}
		cat.ByBundle[payload.BundleID] = append(cat.ByBundle[payload.BundleID], payload.Trait)
		cat.Counts[payload.Trait]++
	}
	for id := range cat.ByBundle {
		sort.Strings(cat.ByBundle[id])
	}
	cat.Names = make([]string, 0, len(cat.Counts))
	for name := range cat.Counts {
		cat.Names = append(cat.Names, name)
	}
	sort.Strings(cat.Names)
	cat.Loaded = true
	return cat
}

// traitAddPost builds the @post for adding a trait, where the trait name is
// a JS EXPRESSION rather than a literal — `evt.target.value` from the select,
// or `$newTrait` from the text input — so the two call sites share it.
func traitAddPost(bundleID, traitExpr string) string {
	return branchkit.MethodPost("app_trait_add", ui.Args("bundle_id", bundleID, "trait", ui.Expr(traitExpr)))
}

// itoa keeps strconv out of the template, which cannot import.
func itoa(n int) string { return strconv.Itoa(n) }

// traitRecordID is the record id, and the format is load-bearing: it is what
// `overrides.apply {action: "remove", id: …}` addresses, so a shipped trait is
// removed by exactly the call that removes a user-added one. Same id space,
// same verb, no shipped-vs-user branch anywhere.
func traitRecordID(trait, bundleID string) string {
	return trait + ":" + bundleID
}

// normalizeTrait lowercases and trims a trait name, and refuses one that
// cannot safely be half of a record id.
//
// The `:` ban is the important one — it is the separator in traitRecordID, so a
// trait containing it would produce an id that parses back wrong. The rest of
// the charset limit is ordinary hygiene: trait names are compared across
// plugins, and a name with spacing or punctuation variants invites two spellings
// of one idea, which is the failure this vocabulary is least able to detect.
func normalizeTrait(raw string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return "", fmt.Errorf("a trait needs a name")
	}
	if len(t) > 32 {
		return "", fmt.Errorf("trait names are at most 32 characters")
	}
	for _, r := range t {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return "", fmt.Errorf("trait names use letters, digits, hyphen and underscore only — %q is not allowed", string(r))
		}
	}
	return t, nil
}

// addAppTrait records that an app has a trait, as the user's own edit.
//
// tenant "_user": this is the user's gesture and the plugin is transport, so it
// must land in the user band — the same reasoning, and the same call, as
// addAppAlias. The user band composes LAST, which is what lets an edit override
// anything shipped.
//
// RESTORE FIRST, and only add if that was not enough. Adding a trait the user
// had previously removed is the common case for a shipped one, and a bare `add`
// gets it subtly wrong: the suppression is cleared, but the user also ends up
// owning a private copy of a row that used to track core_apps.json. A later
// release correcting that trait would then reach everyone EXCEPT the person who
// toggled it off and on — the "frozen at whatever shipped the day you first ran
// it" failure the per-app profiles design went out of its way to avoid.
//
// Measured, not theorised: removing `terminal` from Terminal and re-adding it
// left both a cleared suppression and a redundant user-owned record.
func addAppTrait(p *branchkit.Plugin, bundleID, rawTrait string) error {
	trait, err := normalizeTrait(rawTrait)
	if err != nil {
		return err
	}
	if bundleID == "" {
		return fmt.Errorf("no application given")
	}
	id := traitRecordID(trait, bundleID)

	// Clear a prior suppression. An error here is the ordinary "there was
	// nothing to restore" case, which is not a failure — the add below is what
	// reports one.
	if err := p.Call("overrides.apply", map[string]any{
		"collection": appTraitsCollection, "action": "restore", "tenant": "_user", "id": id,
	}, nil); err != nil {
		branchkit.Logf("system", "trait restore %q (may be a no-op): %v", id, err)
	}

	// If restoring was the whole job, stop. Re-reading is cheap on a settings
	// page and is the only way to tell "this trait ships" from "this trait is
	// new" without the plugin keeping a second copy of the shipped file.
	if hasTrait(p, bundleID, trait) {
		return nil
	}

	if err := p.Call("overrides.apply", map[string]any{
		"collection": appTraitsCollection, "action": "add", "tenant": "_user",
		"fields": map[string]string{"key": id, "trait": trait, "bundle_id": bundleID},
	}, nil); err != nil {
		branchkit.Logf("system", "trait add %q: %v", id, err)
		return fmt.Errorf("could not add trait %q: %w", trait, err)
	}
	return nil
}

// hasTrait reports whether the composed view currently gives this app the trait.
func hasTrait(p *branchkit.Plugin, bundleID, trait string) bool {
	for _, t := range loadTraitCatalog(p).ByBundle[bundleID] {
		if t == trait {
			return true
		}
	}
	return false
}

// removeAppTrait drops a trait from an app.
//
// RESTORE FIRST, then suppress only if the trait survives — the exact mirror of
// addAppTrait, and load-bearing for the same reason.
//
// `restore` is the generic "make this id fall back to base" operation: it
// clears patches, removals AND user-ADDED records for the id
// (collection_override.rs, the "restore" arm). So removing a trait the user
// themselves added simply deletes it, and only a trait still present afterwards
// is a shipped one that needs suppressing.
//
// Plain `remove` alone gets this wrong in a way the composed view hides: it
// appends to `removed` and never touches `added`, so a user-added trait removed
// that way leaves BOTH records sitting on top of each other. The merged result
// is correct, which is exactly why it would go unnoticed — the user's override
// file would grow by a contradictory pair on every add-then-remove cycle,
// forever.
//
// Together the two functions keep one invariant worth stating plainly: THE USER
// BAND HOLDS ONLY WHAT GENUINELY DIFFERS FROM WHAT SHIPS. Put a trait back the
// way it shipped, by any route, and the override disappears entirely.
func removeAppTrait(p *branchkit.Plugin, bundleID, rawTrait string) error {
	trait, err := normalizeTrait(rawTrait)
	if err != nil {
		return err
	}
	if bundleID == "" {
		return fmt.Errorf("no application given")
	}
	id := traitRecordID(trait, bundleID)

	// Drop any user-band state for this id. An error is the ordinary "nothing
	// to restore" case for a trait that only ever came from the shipped file.
	if err := p.Call("overrides.apply", map[string]any{
		"collection": appTraitsCollection, "action": "restore", "tenant": "_user", "id": id,
	}, nil); err != nil {
		branchkit.Logf("system", "trait restore %q (may be a no-op): %v", id, err)
	}

	// Gone already — it was the user's own addition, and restoring deleted it.
	if !hasTrait(p, bundleID, trait) {
		return nil
	}

	// Still there, so it ships. Suppress it.
	if err := p.Call("overrides.apply", map[string]any{
		"collection": appTraitsCollection, "action": "remove", "tenant": "_user", "id": id,
	}, nil); err != nil {
		branchkit.Logf("system", "trait remove %q: %v", id, err)
		return fmt.Errorf("could not remove trait %q: %w", trait, err)
	}
	return nil
}
