package workspace

import (
	"sort"
	"testing"

	"pebblepost/internal/types"
)

func TestParseOrderPrefix(t *testing.T) {
	tests := []struct {
		input       string
		wantOrder   int
		wantClean   string
		wantDisplay string
	}{
		{
			input:       "01-auth",
			wantOrder:   1,
			wantClean:   "auth",
			wantDisplay: "auth",
		},
		{
			input:       "02_users",
			wantOrder:   2,
			wantClean:   "users",
			wantDisplay: "users",
		},
		{
			input:       "10.products",
			wantOrder:   10,
			wantClean:   "products",
			wantDisplay: "products",
		},
		{
			input:       "03-get-profile.pebble.json",
			wantOrder:   3,
			wantClean:   "get-profile.pebble.json",
			wantDisplay: "get-profile",
		},
		{
			input:       "login.pebble.json",
			wantOrder:   0,
			wantClean:   "login.pebble.json",
			wantDisplay: "login",
		},
		{
			input:       "unprefixed-folder",
			wantOrder:   0,
			wantClean:   "unprefixed-folder",
			wantDisplay: "unprefixed-folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			order, clean, display := ParseOrderPrefix(tt.input)
			if order != tt.wantOrder {
				t.Errorf("order = %d, want %d", order, tt.wantOrder)
			}
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if display != tt.wantDisplay {
				t.Errorf("display = %q, want %q", display, tt.wantDisplay)
			}
		})
	}
}

func TestNodeComparator_DirectoriesFirst(t *testing.T) {
	cmp := NewNodeComparator(nil)

	dir := &types.TreeNode{Name: "zebra-folder", IsDir: true}
	file := &types.TreeNode{Name: "01-apple.pebble.json", IsDir: false}

	if !cmp.Compare(dir, file) {
		t.Errorf("expected directory to precede file")
	}
	if cmp.Compare(file, dir) {
		t.Errorf("expected file not to precede directory")
	}
}

func TestNodeComparator_ItemOrderTakesPrecedence(t *testing.T) {
	// Folder specifies explicit sequence: zebra before apple
	cmp := NewNodeComparator([]string{
		"zebra.pebble.json",
		"apple.pebble.json",
	})

	apple := &types.TreeNode{Name: "apple.pebble.json", Order: 1, IsDir: false}
	zebra := &types.TreeNode{Name: "zebra.pebble.json", Order: 99, IsDir: false}

	if !cmp.Compare(zebra, apple) {
		t.Errorf("expected zebra to precede apple due to itemOrder sequence")
	}
}

func TestNodeComparator_ExplicitOrderOverridesPrefix(t *testing.T) {
	cmp := NewNodeComparator(nil)

	// node A has prefix "01-" but explicit Order 10
	nodeA := &types.TreeNode{Name: "01-first.pebble.json", Order: 10, IsDir: false}
	// node B has prefix "02-" but explicit Order 2
	nodeB := &types.TreeNode{Name: "02-second.pebble.json", Order: 2, IsDir: false}

	if !cmp.Compare(nodeB, nodeA) {
		t.Errorf("expected nodeB (order 2) to precede nodeA (order 10)")
	}
}

func TestNodeComparator_NumericPrefixFallback(t *testing.T) {
	cmp := NewNodeComparator(nil)

	node1 := &types.TreeNode{Name: "01-auth", IsDir: true}
	node2 := &types.TreeNode{Name: "02-billing", IsDir: true}
	node3 := &types.TreeNode{Name: "10-users", IsDir: true}

	nodes := []*types.TreeNode{node3, node1, node2}
	sort.Slice(nodes, func(i, j int) bool {
		return cmp.Compare(nodes[i], nodes[j])
	})

	if nodes[0].Name != "01-auth" || nodes[1].Name != "02-billing" || nodes[2].Name != "10-users" {
		t.Errorf("unexpected sort order: %v, %v, %v", nodes[0].Name, nodes[1].Name, nodes[2].Name)
	}
}

func TestNodeComparator_AlphabeticalFallback(t *testing.T) {
	cmp := NewNodeComparator(nil)

	apple := &types.TreeNode{Name: "apple.pebble.json", IsDir: false}
	banana := &types.TreeNode{Name: "banana.pebble.json", IsDir: false}

	if !cmp.Compare(apple, banana) {
		t.Errorf("expected apple before banana")
	}
	if cmp.Compare(banana, apple) {
		t.Errorf("expected banana not before apple")
	}
}
