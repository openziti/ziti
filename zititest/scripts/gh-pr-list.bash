#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

# Set default ZITI_WORKSPACE to grandparent directory if not set
: "${ZITI_WORKSPACE:=$(cd "$(dirname "$0")/../../.." && pwd)}"

# Set default GITHUB_AUTHOR to current GitHub user if authenticated, otherwise fallback to qrkourier
if command -v gh &>/dev/null; then
  : "${GITHUB_AUTHOR:=$(gh api user --jq '.login' 2>/dev/null)}"
  if [ -z "${GITHUB_AUTHOR}" ]; then
    echo "ERROR: gh CLI not logged in. Run 'gh auth login'" >&2
  fi
else
  echo "ERROR: gh command not found. Please install GitHub CLI to use this script." >&2
  exit 1
fi

# Function to display usage information
show_usage() {
  cat <<EOF
Usage: ${0##*/} [DIRECTORY...]

Emit a Markdown list of GitHub PRs that need review from the specified author in the given directories.
If no directories are provided, scans all directories in ZITI_WORKSPACE.

Environment Variables:
  ZITI_WORKSPACE  Base directory to scan when no directories are provided (default: $ZITI_WORKSPACE)
  GITHUB_AUTHOR   GitHub username to filter PRs by author (default: $GITHUB_AUTHOR)

Examples:
  # Scan all repositories in ZITI_WORKSPACE
  ${0##*/}

  # Scan specific repositories
  ${0##*/} /path/to/repo1 /path/to/repo2

  # Use a different GitHub author
  GITHUB_AUTHOR=otheruser ${0##*/}

  # Use a custom workspace directory
  ZITI_WORKSPACE=~/my-repos ${0##*/}
EOF
  exit 0
}

# Show usage if help is requested
if [[ "${1:-}" =~ "--?h(elp)?" ]]; then
  show_usage
fi

# Function to check if a directory is a git repository
is_git_repo() {
  git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1
}

# Function to extract org/repo from git remote URL
get_repo_org_name() {
  local remote_url
  remote_url=$(git -C "$1" config --get remote.origin.url)
  if [[ $remote_url =~ github\.com[/:]([^/]+)/([^/]+)(\.git)$ ]]; then
    echo "${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
  fi
}

# Track if we found any PRs across all directories
FOUND_PR=0
# Track repositories we've processed
REPOS=()
# Track if we're scanning specific repos or organizations
SPECIFIC_REPOS_MODE=0

# Function to process a single directory
process_dir() {
  local dir="$1"
  if is_git_repo "$dir"; then
    repo=$(get_repo_org_name "$dir")
    # Check if repo is set and the org/username contains 'ziti' or 'netfoundry'
    if [ -n "$repo" ] && [[ ${repo%%/*} =~ openziti|netfoundry ]]; then
      # Add to our list of processed repos if we're in specific repos mode
      if [ $SPECIFIC_REPOS_MODE -eq 1 ]; then
        REPOS+=("$repo")
      fi
      # Get PRs and store in a variable to check if there are any results
      local pr_output
      pr_output=$(gh pr list --repo "$repo" --author "$GITHUB_AUTHOR" --search "review:none draft:false sort:updated-asc" --json 'id,title,url,createdAt,additions,deletions' --template '{{ range . }}* [ ] {{ .url }} - {{ .title}} ({{timeago .createdAt}}) (+{{.additions}} -{{.deletions}} lines){{ "\n" }}{{ end }}' 2>/dev/null)
      
      # Only print the heading if there are PRs
      if [ -n "$pr_output" ]; then
        FOUND_PR=1
        echo -e "\n#### $repo ####\n"
        echo "$pr_output"
      fi
    fi
  fi
}

# Function to generate GitHub URL with search parameters
generate_github_url() {
  local base_url="https://github.com/pulls"
  local query="is:open is:pr draft:false review:none author:${GITHUB_AUTHOR} sort:updated-asc"
  
  # If we're in specific repos mode, use the repo filters
  if [ $SPECIFIC_REPOS_MODE -eq 1 ]; then
    local repo_filters=()
    for repo in "${REPOS[@]}"; do
      repo_filters+=("repo:$repo")
    done
    query+=" ${repo_filters[*]}"
  else
    # Otherwise, use organization filters
    query+=" org:openziti org:netfoundry"
  fi
  
  # URL encode the query
  if command -v jq &>/dev/null; then
    local encoded_query=$(echo "$query" | jq -sRr @uri)
    echo "${base_url}?q=${encoded_query}"
  else
    # Fallback to simple encoding that handles spaces
    echo "${base_url}?q=${query// /+}"
  fi
}

# Use provided directories or find all directories in ZITI_WORKSPACE
if [ $# -gt 0 ] && ! [[ "$1" =~ ^- ]]; then
  # We're in specific repos mode
  SPECIFIC_REPOS_MODE=1
  for dir in "$@"; do
    if [ -d "$dir" ]; then
      process_dir "$dir"
    else
      echo "WARN: Directory not found: $dir" >&2
    fi
  done
else
  while read -r dir; do
    process_dir "$dir"
  done < <(find "$ZITI_WORKSPACE" -mindepth 1 -maxdepth 1 -type d)
fi

# Print GitHub URL for the same search criteria if we found any PRs
if (( FOUND_PR )); then
  echo -e "\n#### [Open in browser]($(generate_github_url)) ####"
fi
