package resolver

import (
	"fmt"
	"strings"

	"github.com/ganeshdipdumbare/gale/internal/index"
)

// Node is a vertex in the dependency graph.
type Node struct {
	Name         string
	Package      index.Package
	Dependencies []string
}

// Graph is a dependency DAG.
type Graph struct {
	Nodes map[string]Node
	Order []string
}

// BuildGraph resolves dependencies for the requested packages.
func BuildGraph(idx *index.Index, roots ...string) (*Graph, error) {
	if idx == nil {
		return nil, fmt.Errorf("index not loaded")
	}
	g := &Graph{Nodes: make(map[string]Node)}
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(name string, stack []string) error
	visit = func(name string, stack []string) error {
		key := strings.ToLower(name)
		if visiting[key] {
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(append(stack, name), " -> "), name)
		}
		if visited[key] {
			return nil
		}
		pkg, ok := idx.Get(name)
		if !ok {
			return fmt.Errorf("package %q not found in index", name)
		}
		visiting[key] = true
		stack = append(stack, name)
		for _, dep := range pkg.Dependencies {
			if err := visit(dep, stack); err != nil {
				return err
			}
		}
		visiting[key] = false
		visited[key] = true
		// Only install packages that have bottles; skip virtual/runtime deps.
		if !pkg.HasBottle() {
			return nil
		}
		g.Nodes[key] = Node{
			Name:         pkg.Name,
			Package:      pkg,
			Dependencies: append([]string(nil), pkg.Dependencies...),
		}
		return nil
	}

	for _, root := range roots {
		if err := visit(root, nil); err != nil {
			return nil, err
		}
	}

	order, err := topoSort(g)
	if err != nil {
		return nil, err
	}
	g.Order = order
	return g, nil
}

func topoSort(g *Graph) ([]string, error) {
	inDegree := make(map[string]int, len(g.Nodes))
	for name := range g.Nodes {
		inDegree[name] = 0
	}
	for name, node := range g.Nodes {
		_ = name
		for _, dep := range node.Dependencies {
			key := strings.ToLower(dep)
			if _, ok := g.Nodes[key]; ok {
				inDegree[name]++
			}
		}
	}

	// Recompute in-degree properly
	inDegree = make(map[string]int, len(g.Nodes))
	dependents := make(map[string][]string)
	for name, node := range g.Nodes {
		inDegree[name] = 0
		for _, dep := range node.Dependencies {
			dk := strings.ToLower(dep)
			if _, ok := g.Nodes[dk]; ok {
				inDegree[name]++
				dependents[dk] = append(dependents[dk], name)
			}
		}
	}
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, dep := range dependents[cur] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("dependency cycle detected")
	}
	return order, nil
}

// Levels returns install levels for parallel execution.
func (g *Graph) Levels() [][]string {
	inDegree := make(map[string]int, len(g.Nodes))
	dependents := make(map[string][]string)
	for name, node := range g.Nodes {
		inDegree[name] = 0
		for _, dep := range node.Dependencies {
			dk := strings.ToLower(dep)
			if _, ok := g.Nodes[dk]; ok {
				inDegree[name]++
				dependents[dk] = append(dependents[dk], name)
			}
		}
	}
	var levels [][]string
	remaining := len(g.Nodes)
	for remaining > 0 {
		var level []string
		for name, deg := range inDegree {
			if deg == 0 {
				level = append(level, name)
			}
		}
		if len(level) == 0 {
			break
		}
		levels = append(levels, level)
		for _, name := range level {
			delete(inDegree, name)
			remaining--
			for _, dep := range dependents[name] {
				inDegree[dep]--
			}
		}
	}
	return levels
}

// TreeLines renders the DAG as unicode tree lines for display, rooted at a
// single package.
func (g *Graph) TreeLines(root string) []string {
	return g.MultiTreeLines([]string{root})
}

// MultiTreeLines renders the DAG as unicode tree lines for one or more
// requested roots (e.g. `gale install foo bar`). Each root gets its own
// top-level tree. Shared dependencies that were already fully expanded
// under an earlier branch are shown once and marked "(already shown above)"
// to keep large/diamond-shaped graphs readable and bounded in size.
func (g *Graph) MultiTreeLines(roots []string) []string {
	var lines []string
	shown := make(map[string]bool)

	var walk func(name, prefix string)
	walk = func(name, prefix string) {
		n := g.Nodes[strings.ToLower(name)]
		deps := n.Dependencies
		// Only recurse into dependencies that actually made it into the
		// resolved graph (virtual/no-bottle deps are filtered upstream).
		var resolved []Node
		for _, dep := range deps {
			if dn, ok := g.Nodes[strings.ToLower(dep)]; ok {
				resolved = append(resolved, dn)
			}
		}
		for i, dn := range resolved {
			branch, childPrefix := "├── ", prefix+"│   "
			if i == len(resolved)-1 {
				branch, childPrefix = "└── ", prefix+"    "
			}
			key := strings.ToLower(dn.Name)
			label := dn.Name + " " + dn.Package.Version
			if shown[key] {
				lines = append(lines, prefix+branch+label+" (already shown above)")
				continue
			}
			shown[key] = true
			lines = append(lines, prefix+branch+label)
			walk(dn.Name, childPrefix)
		}
	}

	seenRoot := make(map[string]bool)
	for _, root := range roots {
		key := strings.ToLower(root)
		if seenRoot[key] {
			continue
		}
		seenRoot[key] = true
		node, ok := g.Nodes[key]
		if !ok {
			lines = append(lines, root+" (no bottle for this platform)")
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, node.Name+" "+node.Package.Version)
		shown[key] = true
		walk(node.Name, "")
	}
	if len(lines) == 0 {
		return []string{"(nothing to install)"}
	}
	return lines
}
