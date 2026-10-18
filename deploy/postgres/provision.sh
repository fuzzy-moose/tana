#!/usr/bin/env bash
# Creates one login role and one database owned by it per name, inside a
# running Postgres container, and prints each connection URL once.
# Passwords travel to psql on a pipe only; nothing is written to disk.
set -euo pipefail

usage() {
	echo "usage: $0 [-H url-host] <container> <name>..." >&2
	echo "  url-host defaults to \$PGHOST_FOR_URL or localhost" >&2
	exit 2
}

host="${PGHOST_FOR_URL:-localhost}"
while getopts 'H:' opt; do
	case "$opt" in
	H) host="$OPTARG" ;;
	*) usage ;;
	esac
done
shift $((OPTIND - 1))
[[ $# -ge 2 ]] || usage

container="$1"
shift

# Names are interpolated as identifiers and literals below.
for name in "$@"; do
	if [[ ! "$name" =~ ^[a-z_][a-z0-9_]*$ ]]; then
		echo "invalid name '$name': must match ^[a-z_][a-z0-9_]*\$" >&2
		exit 2
	fi
done

psql() {
	docker exec -i "$container" psql -U postgres -v ON_ERROR_STOP=1 -X -q -t -A "$@"
}

# Base62 keeps the URL free of characters that need escaping in URLs, YAML,
# or .env files. Read a bounded chunk and trim in bash: piping tr into head would SIGPIPE
# tr and abort under pipefail.
password() {
	local pw=""
	while ((${#pw} < 32)); do
		pw+="$(head -c 256 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9')"
	done
	printf '%s' "${pw:0:32}"
}

for name in "$@"; do
	# Separate assignments so a failed check aborts instead of reading as "absent".
	role_exists="$(psql -c "SELECT 1 FROM pg_roles WHERE rolname = '$name'")"
	if [[ -n "$role_exists" ]]; then
		pw='<existing-password>'
		echo "$name: role exists, password unchanged; URL shows a placeholder" >&2
	else
		pw="$(password)"
		printf "CREATE ROLE %s LOGIN PASSWORD '%s';\n" "$name" "$pw" | psql
	fi

	db_exists="$(psql -c "SELECT 1 FROM pg_database WHERE datname = '$name'")"
	if [[ -n "$db_exists" ]]; then
		echo "$name: database exists, not recreated" >&2
	else
		psql -c "CREATE DATABASE $name OWNER $name;"
	fi
	psql -c "REVOKE ALL ON DATABASE $name FROM PUBLIC;"

	echo "postgres://$name:$pw@$host:5432/$name?sslmode=disable"
done
