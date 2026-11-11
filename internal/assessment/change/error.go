// Package change judges one tree against the policy: the built-in and custom
// rules of design sections 6.2 and 6.3 on the check facts of that tree. It
// returns errors, each with the key that names it and every site that breaks
// it. Counting on the base and the head, exceptions and the status of a change
// use those errors and are not decided here.
//
// The package reads no file and runs no tool: it decides over
// internal/model/evidence facts and an internal/policy/resolved policy.
//
// A cycle is keyed by each dependency inside it (rule, from, to), not by the
// sorted module set the milestone 1 plan names: a change that deletes a
// dependency of a cycle leaves a smaller set, and a set key would count that
// set as new. When a change closes a cycle, the record should show first the
// dependency whose site is on an added line; Evaluate does not know added
// lines, so the count on base and head applies that order.
package change

import "github.com/alexei-led/archfit/v3/internal/model/result"

// Error is one rule a tree breaks, with every site of the tree that breaks it
// in the same way.
type Error struct {
	// Key names the error. Errors with equal keys on the base and the head
	// are the same error; its count is the number of sites.
	Key Key
	// From and To are the modules in the order the record shows them: the
	// dependency starts in From. For a dependency rule they are Key.From and
	// Key.To. For duplication From holds the copy the change adds, while the
	// key holds the pair sorted.
	From, To string
	// Sites are the places in the tree. Sites[0] is the site the record
	// shows; the others follow in file and line order.
	Sites []Site
	// What says what is wrong.
	What string
	// Why is the reason the rule exists.
	Why string
	// Instead is what the owner says to do, when the rule has it.
	Instead string
	// Balance is the balance of the dependency, when the rule is about
	// coupling.
	Balance *result.Balance
	// Next is the one step to take.
	Next result.Next
}

// Key identifies an error across trees. It is comparable, so it can key a
// map.
type Key struct {
	// Rule is the rule id.
	Rule string
	// From and To are modules. For a dependency rule and for cycle: the
	// module the dependency starts in and the module it reaches, or for an
	// only rule the library or package set; From is empty for an importer
	// in no module. For duplication: the two modules, sorted. Empty for
	// unowned.
	From, To string
	// Symbol is the imported package or library for a dependency rule, the
	// file for unowned, empty for cycle, and "sha256:" with the hex SHA-256
	// of the literal for duplication.
	Symbol string
}

// Site is a place in a file: a repository-relative, slash-separated path and
// a line. Line 0 is the whole file.
type Site struct {
	File string
	Line int
}
