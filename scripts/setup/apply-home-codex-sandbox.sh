#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
HOME_DIR="${HOME:-}"

if [[ -z "${HOME_DIR}" ]]; then
  echo "HOME is empty; abort."
  exit 1
fi

if [[ ! -d "${PROJECT_ROOT}" ]]; then
  echo "Project root not found: ${PROJECT_ROOT}"
  exit 1
fi

timestamp="$(date +%Y%m%d_%H%M%S)"

backup_if_exists() {
  local file="$1"
  if [[ -f "${file}" ]]; then
    cp "${file}" "${file}.bak.${timestamp}"
    echo "backup: ${file}.bak.${timestamp}"
  fi
}

disable_proxy_autostart() {
  local file="$1"
  [[ -f "${file}" ]] || return 0

  if grep -Eq '^[[:space:]]*enable_proxy[[:space:]]*$' "${file}"; then
    sed -i 's/^[[:space:]]*enable_proxy[[:space:]]*$/# enable_proxy  # disabled by ai-workflow sandbox setup/' "${file}"
    echo "updated: ${file} (proxy autostart disabled)"
  else
    echo "skip: ${file} (no standalone enable_proxy call found)"
  fi
}

upsert_toml_key() {
  local file="$1"
  local section="$2"
  local key="$3"
  local value="$4"
  local tmp
  tmp="$(mktemp)"

  awk -v section="${section}" -v key="${key}" -v value="${value}" '
    function section_name(line, out) {
      out = line
      sub(/^[[:space:]]*\[/, "", out)
      sub(/\][[:space:]]*$/, "", out)
      return out
    }
    BEGIN {
      in_section = 0
      section_found = 0
      key_written = 0
    }
    {
      line = $0
      if (line ~ /^[[:space:]]*\[[^]]+\][[:space:]]*$/) {
        current = section_name(line)
        if (in_section && !key_written) {
          print key " = " value
          key_written = 1
        }
        if (current == section) {
          in_section = 1
          section_found = 1
        } else {
          in_section = 0
        }
        print line
        next
      }

      if (in_section) {
        pattern = "^[[:space:]]*" key "[[:space:]]*="
        if (line ~ pattern) {
          if (!key_written) {
            print key " = " value
            key_written = 1
          }
          next
        }
      }
      print line
    }
    END {
      if (!section_found) {
        if (NR > 0) {
          print ""
        }
        print "[" section "]"
        print key " = " value
      } else if (in_section && !key_written) {
        print key " = " value
      }
    }
  ' "${file}" > "${tmp}"

  mv "${tmp}" "${file}"
}

echo "==> project root: ${PROJECT_ROOT}"
echo "==> home: ${HOME_DIR}"

mkdir -p \
  "${PROJECT_ROOT}/.ai-workflow/home" \
  "${PROJECT_ROOT}/.ai-workflow/tmp" \
  "${PROJECT_ROOT}/.ai-workflow/npm-cache" \
  "${PROJECT_ROOT}/.ai-workflow/xdg-cache" \
  "${PROJECT_ROOT}/.ai-workflow/go-cache" \
  "${PROJECT_ROOT}/.ai-workflow/go-mod-cache" \
  "${PROJECT_ROOT}/.ai-workflow/codex-home"

zshrc="${HOME_DIR}/.zshrc"
bashrc="${HOME_DIR}/.bashrc"
codex_dir="${HOME_DIR}/.codex"
codex_cfg="${codex_dir}/config.toml"

backup_if_exists "${zshrc}"
backup_if_exists "${bashrc}"
mkdir -p "${codex_dir}"
if [[ ! -f "${codex_cfg}" ]]; then
  touch "${codex_cfg}"
fi
backup_if_exists "${codex_cfg}"

disable_proxy_autostart "${zshrc}"
disable_proxy_autostart "${bashrc}"

project_home="${PROJECT_ROOT}/.ai-workflow/home"
project_tmp="${PROJECT_ROOT}/.ai-workflow/tmp"
project_npm_cache="${PROJECT_ROOT}/.ai-workflow/npm-cache"
project_xdg_cache="${PROJECT_ROOT}/.ai-workflow/xdg-cache"
project_go_cache="${PROJECT_ROOT}/.ai-workflow/go-cache"
project_go_mod_cache="${PROJECT_ROOT}/.ai-workflow/go-mod-cache"
project_codex_home="${PROJECT_ROOT}/.ai-workflow/codex-home"

upsert_toml_key "${codex_cfg}" "sandbox_workspace_write" "network_access" "true"
upsert_toml_key "${codex_cfg}" "sandbox_workspace_write" "exclude_slash_tmp" "true"
upsert_toml_key "${codex_cfg}" "sandbox_workspace_write" "exclude_tmpdir_env_var" "true"
upsert_toml_key "${codex_cfg}" "sandbox_workspace_write" "writable_roots" "[]"

upsert_toml_key "${codex_cfg}" "shell_environment_policy" "inherit" "\"core\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "HOME" "\"${project_home}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "TMPDIR" "\"${project_tmp}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "TMP" "\"${project_tmp}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "TEMP" "\"${project_tmp}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "NPM_CONFIG_CACHE" "\"${project_npm_cache}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "npm_config_cache" "\"${project_npm_cache}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "XDG_CACHE_HOME" "\"${project_xdg_cache}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "GOCACHE" "\"${project_go_cache}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "GOMODCACHE" "\"${project_go_mod_cache}\""
upsert_toml_key "${codex_cfg}" "shell_environment_policy.set" "CODEX_HOME" "\"${project_codex_home}\""

project_section="projects.\"${PROJECT_ROOT}\""
upsert_toml_key "${codex_cfg}" "${project_section}" "trust_level" "\"trusted\""

echo "==> done"
echo "Reload shell to apply proxy changes:"
echo "  source ~/.zshrc"
echo "or"
echo "  source ~/.bashrc"
