#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# initialize a file descriptor for debug output
: "${DEBUG:=0}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>/dev/null
fi

# Detect package manager
detect_package_manager() {
  if command -v apt-get &>/dev/null; then
    echo "apt"
  elif command -v dnf &>/dev/null; then
    echo "dnf"
  elif command -v yum &>/dev/null; then
    echo "yum"
  else
    echo "unknown"
  fi
}

# Stop and disable the service
stop_service() {
  echo "Stopping and disabling ziti-controller service..."
  systemctl disable --now ziti-controller.service || true
  systemctl reset-failed ziti-controller.service || true
  systemctl clean --what=state ziti-controller.service || true
}

# Remove configuration files
remove_config_files() {
  echo "Removing configuration files..."
  for FILE in {service,bootstrap}.env
  do
    [[ -f /opt/openziti/etc/controller/${FILE} ]] && rm -f /opt/openziti/etc/controller/${FILE}
  done
}

# Uninstall the package
uninstall_package() {
  local pkg_manager
  pkg_manager=$(detect_package_manager)
  
  echo "Uninstalling openziti-controller package using ${pkg_manager}..."
  
  case "${pkg_manager}" in
    apt)
      apt-get purge -y openziti-controller
      ;;
    dnf)
      dnf remove -y openziti-controller
      ;;
    yum)
      yum remove -y openziti-controller
      ;;
    *)
      echo "ERROR: Unsupported package manager. Please uninstall manually." >&2
      exit 1
      ;;
  esac
}

# Main execution
main() {
  echo "Starting complete removal of openziti-controller..."
  
  stop_service
  remove_config_files
  uninstall_package
  
  echo "openziti-controller has been completely removed from the system."
}

# Execute main function
main "$@"
