package trade

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

// ParseShardWeights reads "office=60,home=40".
func ParseShardWeights(spec string) (map[string]int, error) {
	out := map[string]int{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if !ok || name == "" || err != nil || n <= 0 {
			return nil, fmt.Errorf("shard weight %q: want name=positive number", part)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("shard %q given twice", name)
		}
		out[name] = n
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no shard weights")
	}
	return out, nil
}

// ShardFilter splits scan keys between machines by weight. A key belongs to
// one machine for good (FNV-1a of the key into the weight ranges, names in
// sorted order), so no two machines search the same key and none of them
// needs to know about the others.
func ShardFilter(name string, weights map[string]int) (func(key string) bool, error) {
	if _, ok := weights[name]; !ok {
		return nil, fmt.Errorf("shard %q is not among the weights", name)
	}
	names := make([]string, 0, len(weights))
	total := 0
	for n, w := range weights {
		names = append(names, n)
		total += w
	}
	sort.Strings(names)
	lo := 0
	for _, n := range names {
		if n == name {
			break
		}
		lo += weights[n]
	}
	hi := lo + weights[name]
	return func(key string) bool {
		h := fnv.New32a()
		h.Write([]byte(key))
		slot := int(h.Sum32() % uint32(total))
		return slot >= lo && slot < hi
	}, nil
}
