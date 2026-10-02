package workspace

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"pebblepost/internal/types"
)

var numericPrefixRegex = regexp.MustCompile(`^(\d+)[-_.\s]+(.*)$`)

// ParseOrderPrefix inspects a filename or folder name and extracts any leading
// numeric ordering prefix (e.g., "01-login.pebble.json" -> (1, "login.pebble.json", "login")).
func ParseOrderPrefix(name string) (order int, strippedName string, displayName string) {
	cleanName := name
	displayName = name

	// Strip .pebble.json or .json for display if present
	if strings.HasSuffix(displayName, PebbleExt) {
		displayName = strings.TrimSuffix(displayName, PebbleExt)
	} else if strings.HasSuffix(displayName, ".json") {
		displayName = strings.TrimSuffix(displayName, ".json")
	}

	matches := numericPrefixRegex.FindStringSubmatch(name)
	if len(matches) == 3 {
		parsed, err := strconv.Atoi(matches[1])
		if err == nil {
			order = parsed
			strippedName = matches[2]

			// Strip prefix from display name as well: "01-login" -> "login"
			dispMatches := numericPrefixRegex.FindStringSubmatch(displayName)
			if len(dispMatches) == 3 {
				displayName = dispMatches[2]
			}
			return order, strippedName, displayName
		}
	}

	// No numeric prefix: order is 0 (unset)
	return 0, cleanName, displayName
}

// NodeComparator sorts TreeNodes according to Option C:
// 1. Directories first, then files
// 2. Explicit folder itemOrder list (from _folder.pebble.json) if available
// 3. Explicit node.Order field (from request or folder definition)
// 4. Numeric prefix extracted from filename (e.g., 01-, 02-)
// 5. Case-insensitive alphabetical sort on cleaned name
type NodeComparator struct {
	itemOrderMap map[string]int // child filename/id -> index in _folder.pebble.json
}

// NewNodeComparator creates a comparator optionally configured with a folder's itemOrder.
func NewNodeComparator(itemOrder []string) *NodeComparator {
	orderMap := make(map[string]int)
	for i, item := range itemOrder {
		orderMap[strings.ToLower(item)] = i
	}
	return &NodeComparator{itemOrderMap: orderMap}
}

// Compare returns true if a should be ordered before b.
func (c *NodeComparator) Compare(a, b *types.TreeNode) bool {
	// 1. Directories before files
	if a.IsDir != b.IsDir {
		return a.IsDir
	}

	// 2. Check _folder.pebble.json itemOrder sequence if configured
	if len(c.itemOrderMap) > 0 {
		aBase := strings.ToLower(filepath.Base(a.Name))
		bBase := strings.ToLower(filepath.Base(b.Name))
		aIdx, aInOrder := c.itemOrderMap[aBase]
		bIdx, bInOrder := c.itemOrderMap[bBase]

		if aInOrder && bInOrder {
			return aIdx < bIdx
		}
		if aInOrder {
			return true
		}
		if bInOrder {
			return false
		}
	}

	// 3 & 4. Compute effective order: explicit Order > numeric prefix
	aOrder := a.Order
	if aOrder == 0 {
		aOrder, _, _ = ParseOrderPrefix(a.Name)
	}

	bOrder := b.Order
	if bOrder == 0 {
		bOrder, _, _ = ParseOrderPrefix(b.Name)
	}

	if aOrder > 0 && bOrder > 0 && aOrder != bOrder {
		return aOrder < bOrder
	}
	if aOrder > 0 && bOrder == 0 {
		return true
	}
	if aOrder == 0 && bOrder > 0 {
		return false
	}

	// 5. Alphabetical fallback on stripped name
	_, aStripped, _ := ParseOrderPrefix(a.Name)
	_, bStripped, _ := ParseOrderPrefix(b.Name)

	return strings.ToLower(aStripped) < strings.ToLower(bStripped)
}
