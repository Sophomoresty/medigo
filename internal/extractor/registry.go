package extractor

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
)

var (
	mu         sync.RWMutex
	extractors []registeredExtractor
	sites      []SiteInfo
)

type registeredExtractor struct {
	patterns []*regexp.Regexp
	ext      Extractor
}

func Register(ext Extractor, info SiteInfo) {
	mu.Lock()
	defer mu.Unlock()

	var compiled []*regexp.Regexp
	for _, p := range ext.Patterns() {
		re, err := regexp.Compile(p)
		if err != nil {
			panic(fmt.Sprintf("extractor %s: invalid pattern %q: %v", info.Name, p, err))
		}
		compiled = append(compiled, re)
	}
	extractors = append(extractors, registeredExtractor{
		patterns: compiled,
		ext:      ext,
	})
	sites = append(sites, info)
}

func Match(url string) (Extractor, error) {
	ext, _, err := MatchWithSite(url)
	return ext, err
}

// MatchWithSite returns both the extractor and its registered site metadata.
func MatchWithSite(url string) (Extractor, SiteInfo, error) {
	mu.RLock()
	defer mu.RUnlock()

	for i, re := range extractors {
		for _, p := range re.patterns {
			if p.MatchString(url) {
				if i < len(sites) {
					return re.ext, sites[i], nil
				}
				return re.ext, SiteInfo{}, nil
			}
		}
	}
	return nil, SiteInfo{}, fmt.Errorf("no extractor found for URL: %s", url)
}

func ListSites() []SiteInfo {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]SiteInfo, len(sites))
	copy(result, sites)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
