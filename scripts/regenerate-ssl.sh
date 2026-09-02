#!/bin/bash
# regenerate-ssl.sh — Regenerates wildcard SSL cert for all active sites.
# Run automatically on site create/delete to keep cert SANs in sync.

set -e

SITES=$(sqlite3 /root/SigardaPanel-Enterprise/sigardapanel.db "SELECT domain FROM sites WHERE status='active';" 2>/dev/null || echo "")

# Collect unique domains and wildcard TLDs
DOMAINS=""
WILDCARDS=""

for domain in $SITES; do
  [ -z "$domain" ] && continue
  
  # Add explicit domain
  DOMAINS="$DOMAINS,DNS:$domain"
  
  # Extract base TLD (second-level domain + TLD)
  tld=$(echo "$domain" | sed -E 's/^[^.]+\.//')
  if [ -n "$tld" ] && ! echo "$WILDCARDS" | grep -q "DNS:*.$tld"; then
    WILDCARDS="$WILDCARDS,DNS:*.$tld"
  fi
done

# Remove leading comma
DOMAINS="${DOMAINS#,}"
WILDCARDS="${WILDCARDS#,}"

# Ensure panel and base wildcards always included
ALL_SANS="DNS:panel.sigarda.dev${DOMAINS:+,$DOMAINS}${WILDCARDS:+,$WILDCARDS}"

# Remove duplicates
ALL_SANS=$(echo "$ALL_SANS" | tr ',' '\n' | sort -u | tr '\n' ',' | sed 's/,$//' | tr -d ' ')

echo "Regenerating cert with SANs: $ALL_SANS"

openssl req -x509 -nodes -newkey rsa:4096 -keyout /etc/nginx/ssl/sigardapanel.key \
  -out /etc/nginx/ssl/sigardapanel.crt -sha256 -days 3650 \
  -subj "/CN=panel.sigarda.dev" \
  -addext "subjectAltName=${ALL_SANS}" 2>/dev/null

# Verify
COUNTS=$(openssl x509 -in /etc/nginx/ssl/sigardapanel.crt -text -noout | grep -o "DNS:" | wc -l)
echo "✅ Cert regenerated with $COUNTS SAN entries"
