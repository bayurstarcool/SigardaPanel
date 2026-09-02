#!/usr/bin/env bash
# SigardaPanel SSH dashboard
# Commands: sigardapanel-info | sigardapanel-info --watch | sigardapanel-info --show-db-pass

# Keep non-interactive shells clean. The file remains directly executable.
if [[ "${BASH_SOURCE[0]}" != "$0" && $- != *i* ]]; then
    return 0
fi

_sigarda_dashboard() {
    local watch_mode=0 show_password=0 interval=2 arg
    for arg in "$@"; do
        case "$arg" in
            --watch|-w) watch_mode=1 ;;
            --show-db-pass|-p) show_password=1 ;;
            --help|-h) printf 'Usage: sigardapanel-info [--watch] [--show-db-pass]\n'; return 0 ;;
        esac
    done

    local reset='' bold='' dim='' white='' muted='' violet='' green='' yellow='' red='' cyan=''
    if [[ -t 1 && "${TERM:-dumb}" != dumb ]] && command -v tput >/dev/null 2>&1; then
        reset=$(tput sgr0); bold=$(tput bold); dim=$(tput dim)
        white=$(tput setaf 255); muted=$(tput setaf 245); violet=$(tput setaf 141)
        green=$(tput setaf 42); yellow=$(tput setaf 220); red=$(tput setaf 203); cyan=$(tput setaf 81)
    fi

    local env_file=${SIGARDAPANEL_ENV_FILE:-/etc/sigardapanel/panel.env}
    if [[ -r "$env_file" ]]; then
        while IFS='=' read -r key value; do
            case "$key" in
                SIGARDAPANEL_DB_PATH) [[ -n "$value" ]] && export SIGARDAPANEL_DB_PATH=$value ;;
                SIGARDAPANEL_API_ADDR) [[ -n "$value" ]] && export SIGARDAPANEL_API_ADDR=$value ;;
                SIGARDAPANEL_AGENT_ADDR) [[ -n "$value" ]] && export SIGARDAPANEL_AGENT_ADDR=$value ;;
            esac
        done < "$env_file"
    fi
    local db=${SIGARDAPANEL_DB_PATH:-/var/lib/sigardapanel/sigardapanel.db}
    local bin=${SIGARDAPANEL_BIN:-$(command -v sigardapanel 2>/dev/null || printf '/usr/local/bin/sigardapanel')}

    _sigarda_age() {
        local pid=$1 elapsed
        elapsed=$(ps -o etimes= -p "$pid" 2>/dev/null | tr -d ' ')
        [[ "$elapsed" =~ ^[0-9]+$ ]] || return 0
        if (( elapsed >= 86400 )); then printf '%dd' "$((elapsed/86400))"
        elif (( elapsed >= 3600 )); then printf '%dh' "$((elapsed/3600))"
        elif (( elapsed >= 60 )); then printf '%dm' "$((elapsed/60))"
        else printf '%ds' "$elapsed"; fi
    }

    _sigarda_http_status() {
        local name=$1 port=$2 path=${3:-/health} code pid age state color
        code=$(curl -sS -o /dev/null --connect-timeout 0.4 --max-time 1 -w '%{http_code}' "http://127.0.0.1:${port}${path}" 2>/dev/null || true)
        pid=$(ss -H -lntp "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -n1)
        if [[ "$code" =~ ^(2|3)[0-9][0-9]$ ]]; then state='ONLINE'; color=$green
        elif [[ -n "$pid" ]]; then state='DEGRADED'; color=$yellow
        else state='OFFLINE'; color=$red; fi
        [[ -n "$pid" ]] && age=$(_sigarda_age "$pid")
        printf '  %-10s %s● %-8s%s  %s:%-5s%s %s%s%s\n' "$name" "$color" "$state" "$reset" "$dim" "$port" "$reset" "$muted" "${age:+up $age}" "$reset"
    }

    _sigarda_socket_status() {
        local name=$1 ports=$2 display=$3 port pid='' first_pid='' age state color all_online=1
        IFS=',' read -ra _sigarda_ports <<< "$ports"
        for port in "${_sigarda_ports[@]}"; do
            pid=$(ss -H -lntp "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -n1)
            [[ -n "$first_pid" ]] || first_pid=$pid
            [[ -n "$pid" ]] || all_online=0
        done
        if (( all_online )); then state='ONLINE'; color=$green; age=$(_sigarda_age "$first_pid")
        else state='OFFLINE'; color=$red; fi
        printf '  %-10s %s● %-8s%s  %s%-6s%s %s%s%s\n' "$name" "$color" "$state" "$reset" "$dim" "$display" "$reset" "$muted" "${age:+up $age}" "$reset"
    }

    _sigarda_bar() {
        local pct=${1:-0} color=$2 filled empty i bar=''
        [[ "$pct" =~ ^[0-9]+$ ]] || pct=0; (( pct > 100 )) && pct=100
        filled=$((pct / 5)); empty=$((20 - filled))
        for ((i=0; i<filled; i++)); do bar+='━'; done
        for ((i=0; i<empty; i++)); do bar+='─'; done
        printf '%s%s%s' "$color" "$bar" "$reset"
    }

    _sigarda_render() {
        local version host ip now uptime_text active_sites total_sites processes load
        local mem_used mem_total mem_pct disk_used disk_total disk_pct mem_color disk_color
        version=$($bin version 2>/dev/null || printf '?')
        host=$(hostname -s 2>/dev/null || printf 'server')
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
        now=$(date '+%a, %d %b %Y  %H:%M:%S %Z')
        uptime_text=$(uptime -p 2>/dev/null | sed 's/^up //' || printf '?')
        active_sites=$(sqlite3 "$db" "SELECT COUNT(*) FROM sites WHERE status='active';" 2>/dev/null || printf 0)
        total_sites=$(sqlite3 "$db" "SELECT COUNT(*) FROM sites;" 2>/dev/null || printf 0)
        processes=$(ps -e --no-headers 2>/dev/null | wc -l | tr -d ' ')
        load=$(awk '{printf "%.2f  %.2f  %.2f",$1,$2,$3}' /proc/loadavg 2>/dev/null || printf '?')
        read -r mem_used mem_total mem_pct < <(free -m 2>/dev/null | awk '/Mem:/{printf "%.1fG %.1fG %d",$3/1024,$2/1024,$3/$2*100}')
        read -r disk_used disk_total disk_pct < <(df -h / 2>/dev/null | awk 'NR==2{gsub(/%/,"",$5); print $3,$2,$5}')
        mem_color=$green; (( ${mem_pct:-0} >= 70 )) && mem_color=$yellow; (( ${mem_pct:-0} >= 90 )) && mem_color=$red
        disk_color=$green; (( ${disk_pct:-0} >= 75 )) && disk_color=$yellow; (( ${disk_pct:-0} >= 90 )) && disk_color=$red

        (( watch_mode )) && printf '\033[H\033[2J'
        printf '\n  %s%sSIGARDAPANEL%s  %sCONTROL PLANE%s\n' "$bold" "$violet" "$reset" "$dim" "$reset"
        printf '  %s%s%s  %s•%s  v%s  %s•%s  %s\n' "$white" "$host" "$reset" "$dim" "$reset" "$version" "$dim" "$reset" "${ip:-no address}"
        printf '  %s%s%s\n\n' "$muted" "$now" "$reset"

        printf '  %s%sSERVICES%s\n' "$bold" "$white" "$reset"
        _sigarda_http_status 'API' 7700 /health
        _sigarda_http_status 'Agent' 7710 /health
        _sigarda_http_status 'Web' 7720 /
        _sigarda_socket_status 'Nginx' '80,443' '80/443'
        _sigarda_socket_status 'MariaDB' '3306' '3306'

        printf '\n  %s%sSYSTEM%s\n' "$bold" "$white" "$reset"
        printf '  %-10s %s%s%s\n' 'Uptime' "$cyan" "$uptime_text" "$reset"
        printf '  %-10s %s%s active%s  %s/ %s total%s\n' 'Sites' "$green" "$active_sites" "$reset" "$dim" "$total_sites" "$reset"
        printf '  %-10s %s%s%s\n' 'Processes' "$white" "$processes" "$reset"
        printf '  %-10s %s%s%s\n' 'Load' "$white" "$load" "$reset"
        printf '  %-10s ' 'Memory'; _sigarda_bar "${mem_pct:-0}" "$mem_color"; printf '  %s%3s%%%s  %s%s / %s%s\n' "$mem_color" "${mem_pct:-0}" "$reset" "$dim" "${mem_used:-?}" "${mem_total:-?}" "$reset"
        printf '  %-10s ' 'Disk'; _sigarda_bar "${disk_pct:-0}" "$disk_color"; printf '  %s%3s%%%s  %s%s / %s%s\n' "$disk_color" "${disk_pct:-0}" "$reset" "$dim" "${disk_used:-?}" "${disk_total:-?}" "$reset"

        printf '\n  %stmux attach -t sigarda%s' "$dim" "$reset"
        (( watch_mode )) && printf '  %s• refreshing every %ss • Ctrl+C to exit%s' "$dim" "$interval" "$reset"
        printf '\n\n'

        if (( show_password )); then
            local db_password
            db_password=$(< /etc/sigardapanel/mariadb_root_password 2>/dev/null || printf 'not set')
            printf '  %sMariaDB root password:%s %s%s%s\n\n' "$muted" "$reset" "$yellow" "$db_password" "$reset"
        fi
    }

    if (( watch_mode )); then
        trap 'printf "\033[?25h"; return 0 2>/dev/null || exit 0' INT TERM
        printf '\033[?25l'
        while :; do _sigarda_render; sleep "$interval"; done
    else _sigarda_render; fi
}

sigardapanel-info() { _sigarda_dashboard "$@"; }

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    _sigarda_dashboard "$@"
elif [[ $- == *i* && -z "${SIGARDAPANEL_INFO_SHOWN:-}" ]]; then
    export SIGARDAPANEL_INFO_SHOWN=1
    _sigarda_dashboard
fi
