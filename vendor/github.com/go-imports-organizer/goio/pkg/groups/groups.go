package groups

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	v1alpha1 "github.com/go-imports-organizer/goio/pkg/api/v1alpha1"
	"github.com/go-imports-organizer/goio/pkg/sorter"
)

// Build asembles the RegExpMatchers that are used to group imports and the
// array that defines the display order for the groups in the import block
func Build(groups []v1alpha1.Group, goModuleName string) ([]v1alpha1.RegExpMatcher, []string) {
	groupRegExpMatchers := []v1alpha1.RegExpMatcher{}
	displayOrder := []string{}

	for _, group := range groups {
		displayOrder = append(displayOrder, group.Description)
	}

	sort.Sort(sorter.SortGroupsByMatchOrder(groups))

	for i := range groups {
		r := strings.Join(groups[i].RegExp, "|")
		r = strings.Replace(r, `%{module}%`, fmt.Sprintf("^%s", strings.ReplaceAll(strings.ReplaceAll(goModuleName, `.`, `\.`), `/`, `\/`)), -1)
		groupRegExpMatchers = append(groupRegExpMatchers, v1alpha1.RegExpMatcher{
			Bucket: groups[i].Description,
			RegExp: regexp.MustCompile(r),
		},
		)
	}
	return groupRegExpMatchers, displayOrder
}
