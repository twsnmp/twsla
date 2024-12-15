#!/usr/bin/env sh

set -eu
printf '\n'

BOLD="$(tput bold 2>/dev/null || printf '')"
GREY="$(tput setaf 0 2>/dev/null || printf '')"
UNDERLINE="$(tput smul 2>/dev/null || printf '')"
RED="$(tput setaf 1 2>/dev/null || printf '')"
GREEN="$(tput setaf 2 2>/dev/null || printf '')"
YELLOW="$(tput setaf 3 2>/dev/null || printf '')"
BLUE="$(tput setaf 4 2>/dev/null || printf '')"
MAGENTA="$(tput setaf 5 2>/dev/null || printf '')"
NO_COLOR="$(tput sgr0 2>/dev/null || printf '')"

SUPPORTED_TARGETS="Linux_x86_64 Linux_arm64 \
                  Darwin_x86_64 Darwin_arm64"
info() {
  printf '%s\n' "${BOLD}${GREY}>${NO_COLOR} $*"
}

warn() {
  printf '%s\n' "${YELLOW}! $*${NO_COLOR}"
}

error() {
  printf '%s\n' "${RED}x $*${NO_COLOR}" >&2
}

completed() {
  printf '%s\n' "${GREEN}✓${NO_COLOR} $*"
}

has() {
  command -v "$1" 1>/dev/null 2>&1
}

get_tmpfile() {
  suffix="$1"
  if has mktemp; then
    printf "%s.%s" "$(mktemp)" "${suffix}"
  else
    printf "/tmp/twsla.%s" "${suffix}"
  fi
}

test_writeable() {
  path="${1:-}/test.txt"
  if touch "${path}" 2>/dev/null; then
    rm "${path}"
    return 0
  else
    return 1
  fi
}

download() {
  file="$1"
  url="$2"

  if has curl ; then
    cmd="curl --fail --silent --location --output $file $url"
  elif has wget; then
    cmd="wget --quiet --output-document=$file $url"
  elif has fetch; then
    cmd="fetch --quiet --output=$file $url"
  else
    error "No HTTP download program (curl, wget, fetch) found, exiting…"
    return 1
  fi

  $cmd && return 0 || rc=$?

  error "Command failed (exit code $rc): ${BLUE}${cmd}${NO_COLOR}"
  printf "\n" >&2
  info "This is likely due to twsla not yet supporting your configuration."
  info "If you would like to see a build for your configuration,"
  info "please create an issue requesting a build for ${MAGENTA}${TARGET}${NO_COLOR}:"
  info "${BOLD}${UNDERLINE}https://github.com/twsnmp/twsla/issues/new/${NO_COLOR}"
  return $rc
}

unpack() {
  archive=$1
  bin_dir=$2
  sudo=${3-}

  case "$archive" in
    *.tar.gz)
      flags=$(test -n "${VERBOSE-}" && echo "-xzvof" || echo "-xzof")
      ${sudo} tar "${flags}" "${archive}" -C "${bin_dir}"
      return 0
      ;;
    *.zip)
      flags=$(test -z "${VERBOSE-}" && echo "-qqo" || echo "-o")
      UNZIP="${flags}" ${sudo} unzip "${archive}" -d "${bin_dir}"
      return 0
      ;;
  esac

  error "Unknown package extension."
  printf "\n"
  info "This almost certainly results from a bug in this script--please file a"
  info "bug report at https://github.com/twsnmp/twsla/issues"
  return 1
}

usage() {
  printf "%s\n" \
    "install.sh [option]" \
    "" \
    "Fetch and install the latest version of twsla, if twsla is already" \
    "installed it will be updated to the latest version."

  printf "\n%s\n" "Options"
  printf "\t%s\n\t\t%s\n\n" \
    "-V, --verbose" "Enable verbose output for the installer" \
    "-b, --bin-dir" "Override the bin installation directory [default: ${BIN_DIR}]" \
    "-v, --version" "Install a specific version of twsla [default: ${VERSION}]" \
    "-h, --help" "Display this help message"
}

elevate_priv() {
  if ! has sudo; then
    error 'Could not find the command "sudo", needed to get permissions for install.'
    info "If you are on Windows, please run your shell as an administrator, then"
    info "rerun this script. Otherwise, please run this script as root, or install"
    info "sudo."
    exit 1
  fi
  if ! sudo -v; then
    error "Superuser not granted, aborting installation"
    exit 1
  fi
}

install() {
  ext="$1"

  if test_writeable "${BIN_DIR}"; then
    sudo=""
    msg="Installing twsla, please wait…"
  else
    warn "Escalated permissions are required to install to ${BIN_DIR}"
    elevate_priv
    sudo="sudo"
    msg="Installing twsla as root, please wait…"
  fi
  info "$msg"

  archive=$(get_tmpfile "$ext")

  download "${archive}" "${URL}"

  unpack "${archive}" "${BIN_DIR}" "${sudo}"
}

# Currently supporting:
#   - darwin
#   - linux
detect_platform() {
  platform="$(uname -s | tr '[:upper:]' '[:lower:]')"

  case "${platform}" in
    linux) platform="Linux" ;;
    darwin) platform="Darwin" ;;
  esac

  printf '%s' "${platform}"
}

# Currently supporting:
#   - x86_64
#   - arm64
detect_arch() {
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"

  case "${arch}" in
    amd64) arch="x86_64" ;;
    aarch64) arch="arm64" ;;
  esac
  printf '%s' "${arch}"
}

detect_target() {
  arch="$1"
  platform="$2"
  target="${platform}_${arch}"

  printf '%s' "${target}"
}

confirm() {
  if [ -z "${FORCE-}" ]; then
    printf "%s " "${MAGENTA}?${NO_COLOR} $* ${BOLD}[y/N]${NO_COLOR}"
    set +e
    read -r yn </dev/tty
    rc=$?
    set -e
    if [ $rc -ne 0 ]; then
      error "Error reading from prompt (please re-run with the '--yes' option)"
      exit 1
    fi
    if [ "$yn" != "y" ] && [ "$yn" != "yes" ]; then
      error 'Aborting (please answer "yes" to continue)'
      exit 1
    fi
  fi
}

check_bin_dir() {
  bin_dir="${1%/}"

  if [ ! -d "$BIN_DIR" ]; then
    error "Installation location $BIN_DIR does not appear to be a directory"
    info "Make sure the location exists and is a directory, then try again."
    usage
    exit 1
  fi

  good=$(
    IFS=:
    for path in $PATH; do
      if [ "${path%/}" = "${bin_dir}" ]; then
        printf 1
        break
      fi
    done
  )

  if [ "${good}" != "1" ]; then
    warn "Bin directory ${bin_dir} is not in your \$PATH"
  fi
}

is_build_available() {
  arch="$1"
  platform="$2"
  target="$3"

  good=$(
    IFS=" "
    for t in $SUPPORTED_TARGETS; do
      if [ "${t}" = "${target}" ]; then
        printf 1
        break
      fi
    done
  )

  if [ "${good}" != "1" ]; then
    error "${arch} builds for ${platform} are not yet available for twsla"
    printf "\n" >&2
    info "If you would like to see a build for your configuration,"
    info "please create an issue requesting a build for ${MAGENTA}${target}${NO_COLOR}:"
    info "${BOLD}${UNDERLINE}https://github.com/twsnmp/twsla/issues/new/${NO_COLOR}"
    printf "\n"
    exit 1
  fi
}

# defaults
if [ -z "${PLATFORM-}" ]; then
  PLATFORM="$(detect_platform)"
fi

if [ -z "${BIN_DIR-}" ]; then
  BIN_DIR=/usr/local/bin
fi

if [ -z "${ARCH-}" ]; then
  ARCH="$(detect_arch)"
fi

if [ -z "${BASE_URL-}" ]; then
  BASE_URL="https://github.com/twsnmp/twsla/releases"
fi

if [ -z "${VERSION-}" ]; then
  VERSION="latest"
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
  -b | --bin-dir)
    BIN_DIR="$2"
    shift 2
    ;;
  -v | --version)
    VERSION="$2"
    shift 2
    ;;

  -V | --verbose)
    VERBOSE=1
    shift 1
    ;;
  -h | --help)
    usage
    exit
    ;;

  -b=* | --bin-dir=*)
    BIN_DIR="${1#*=}"
    shift 1
    ;;
  -v=* | --version=*)
    VERSION="${1#*=}"
    shift 1
    ;;
  -V=* | --verbose=*)
    VERBOSE="${1#*=}"
    shift 1
    ;;

  *)
    error "Unknown option: $1"
    usage
    exit 1
    ;;
  esac
done

TARGET="$(detect_target "${ARCH}" "${PLATFORM}")"

is_build_available "${ARCH}" "${PLATFORM}" "${TARGET}"

printf "  %s\n" "${UNDERLINE}Configuration${NO_COLOR}"
info "${BOLD}Bin directory${NO_COLOR}: ${GREEN}${BIN_DIR}${NO_COLOR}"
info "${BOLD}Platform${NO_COLOR}:      ${GREEN}${PLATFORM}${NO_COLOR}"
info "${BOLD}Arch${NO_COLOR}:          ${GREEN}${ARCH}${NO_COLOR}"

if [ -n "${VERBOSE-}" ]; then
  VERBOSE=v
  info "${BOLD}Verbose${NO_COLOR}: yes"
else
  VERBOSE=
fi

printf '\n'

EXT=tar.gz
if [ "${VERSION}" != "latest" ]; then
  URL="${BASE_URL}/download/${VERSION}/twsla_${TARGET}.${EXT}"
else
  URL="${BASE_URL}/latest/download/twsla_${TARGET}.${EXT}"
fi
info "Tarball URL: ${UNDERLINE}${BLUE}${URL}${NO_COLOR}"
confirm "Install twsla ${GREEN}${VERSION}${NO_COLOR} to ${BOLD}${GREEN}${BIN_DIR}${NO_COLOR}?"
check_bin_dir "${BIN_DIR}"
install "${EXT}"
completed "twsla ${VERSION} installed"

