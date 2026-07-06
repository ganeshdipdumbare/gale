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

// TreeLines renders the DAG as unicode tree lines for display.
func (g *Graph) TreeLines(root string) []string {
	key := strings.ToLower(root)
	node, ok := g.Nodes[key]
	if !ok {
		return []string{root}
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s", node.Name, node.Package.Version))
	var walk func(name, prefix string, last bool)
	walk = func(name, prefix string, last bool) {
		n := g.Nodes[strings.ToLower(name)]
		deps := n.Dependencies
		for i, dep := range deps {
			dk := strings.ToLower(dep)
			dn, ok := g.Nodes[dk]
			if !ok {
				continue
			}
			branch := "├── "
			childPrefix := prefix + "│   "
			if i == len(deps)-1 {
				branch = "└── "
				childPrefix = prefix + "    "
			}
			lines = append(lines, prefix+branch+dn.Name+" "+dn.Package.Version)
			walk(dn.Name, childPrefix, i == len(deps)-1)
		}
	}
	walk(node.Name, "", true)
	return lines
}
