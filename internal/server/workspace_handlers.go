package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nchungdev/agent-bridge/internal/bridge"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"is_dir"`
	Size     int64       `json:"size,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// handleGetFileTree scans a directory and returns a recursive file tree
func handleGetFileTree(w http.ResponseWriter, r *http.Request) {
	rootPath := r.URL.Query().Get("path")
	if rootPath == "" {
		rootPath = bridge.DefaultWorkspacePath()
	}

	// Security check: ensure path is within safe boundaries
	rootPath = filepath.Clean(rootPath)
	if !PathAllowed(rootPath) {
		forbidPath(w)
		return
	}

	tree, err := buildFileTree(rootPath, 3) // depth 3 to stay fast
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	jsonResponse(w, tree)
}

func buildFileTree(dir string, maxDepth int) (*FileNode, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}

	node := &FileNode{
		Name:  filepath.Base(dir),
		Path:  dir,
		IsDir: info.IsDir(),
	}

	if !info.IsDir() || maxDepth <= 0 {
		node.Size = info.Size()
		return node, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return node, nil // ignore permission errors
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden, git, and heavy dependency directories
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == ".agent" {
			continue
		}

		childPath := filepath.Join(dir, name)
		if entry.IsDir() {
			childNode, _ := buildFileTree(childPath, maxDepth-1)
			if childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		} else {
			eInfo, _ := entry.Info()
			var sz int64
			if eInfo != nil {
				sz = eInfo.Size()
			}
			node.Children = append(node.Children, &FileNode{
				Name:  name,
				Path:  childPath,
				IsDir: false,
				Size:  sz,
			})
		}
	}

	// Sort: Directories first, then alphabetical
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})

	return node, nil
}

type GitStatusResponse struct {
	RepoName  string   `json:"repo_name"`
	Branch    string   `json:"branch"`
	Files     []string `json:"files"`
	Diff      string   `json:"diff"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	HasDiff   bool     `json:"has_diff"`
}

// handleGetGitDiff returns the current git status and unified diff for the workspace
func handleGetGitDiff(w http.ResponseWriter, r *http.Request) {
	wsPath := r.URL.Query().Get("path")
	if wsPath == "" {
		wsPath = bridge.DefaultWorkspacePath()
	}
	wsPath = filepath.Clean(wsPath)
	if !PathAllowed(wsPath) {
		forbidPath(w)
		return
	}

	// If wsPath is not a git repo, check if any immediate subproject has a .git
	actualRepo := wsPath
	if _, err := os.Stat(filepath.Join(wsPath, ".git")); err != nil {
		// Look for first subfolder with .git
		entries, _ := os.ReadDir(wsPath)
		for _, e := range entries {
			if e.IsDir() {
				sub := filepath.Join(wsPath, e.Name())
				if _, err := os.Stat(filepath.Join(sub, ".git")); err == nil {
					actualRepo = sub
					break
				}
				// Also check projects/ subdirectory
				if e.Name() == "projects" {
					subEntries, _ := os.ReadDir(sub)
					for _, se := range subEntries {
						if se.IsDir() {
							subsub := filepath.Join(sub, se.Name())
							if _, err := os.Stat(filepath.Join(subsub, ".git")); err == nil {
								actualRepo = subsub
								break
							}
						}
					}
				}
			}
		}
	}

	// Check branch
	cmdBranch := exec.Command("git", "-C", actualRepo, "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := cmdBranch.Output()
	branch := strings.TrimSpace(string(branchBytes))
	if err != nil {
		branch = filepath.Base(actualRepo)
	}

	// Git status short
	cmdStatus := exec.Command("git", "-C", actualRepo, "status", "--porcelain")
	statusBytes, _ := cmdStatus.Output()
	statusLines := strings.Split(strings.TrimSpace(string(statusBytes)), "\n")
	var changedFiles []string
	for _, l := range statusLines {
		if strings.TrimSpace(l) != "" {
			changedFiles = append(changedFiles, strings.TrimSpace(l))
		}
	}

	// Git diff
	cmdDiff := exec.Command("git", "-C", actualRepo, "diff", "HEAD")
	diffBytes, err := cmdDiff.Output()
	diff := ""
	if err == nil {
		diff = string(diffBytes)
	}

	// Fallback to unstaged diff if HEAD diff was empty
	if strings.TrimSpace(diff) == "" {
		cmdDiffUnstaged := exec.Command("git", "-C", actualRepo, "diff")
		diffUnstagedBytes, _ := cmdDiffUnstaged.Output()
		diff = string(diffUnstagedBytes)
	}

	var additions, deletions int
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			additions++
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			deletions++
		}
	}

	repoName := filepath.Base(actualRepo)

	res := GitStatusResponse{
		RepoName:  repoName,
		Branch:    branch,
		Files:     changedFiles,
		Diff:      diff,
		Additions: additions,
		Deletions: deletions,
		HasDiff:   len(changedFiles) > 0 || len(strings.TrimSpace(diff)) > 0,
	}

	jsonResponse(w, res)
}

// handleGetFileContent reads the content of a file
func handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	if !PathAllowed(filepath.Clean(filePath)) {
		forbidPath(w)
		return
	}
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}
