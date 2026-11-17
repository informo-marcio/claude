#!/bin/sh
# Backup do banco do portal com pg_dump em formato custom.
#
#   backup.sh now    faz um backup agora e sai
#   backup.sh loop   faz um backup por dia na hora BACKUP_HOUR (padrão 2h)
#
# Usa as variáveis padrão do PostgreSQL (PGHOST, PGUSER, PGDATABASE,
# PGPASSWORD). Apaga os backups com mais de BACKUP_RETENTION_DAYS dias.
set -eu

BACKUP_DIR="${BACKUP_DIR_IN_CONTAINER:-/backups}"
HOUR="${BACKUP_HOUR:-2}"
KEEP="${BACKUP_RETENTION_DAYS:-30}"

case "$HOUR" in
[0-9] | 1[0-9] | 2[0-3]) ;;
*) echo "BACKUP_HOUR deve estar entre 0 e 23" >&2; exit 2 ;;
esac
case "$KEEP" in
'' | *[!0-9]*) echo "BACKUP_RETENTION_DAYS deve ser um número de dias" >&2; exit 2 ;;
esac

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') backup: $*"; }

backup_now() {
	mkdir -p "$BACKUP_DIR"
	name="payback-$(date '+%Y%m%d-%H%M%S').dump"
	tmp="$BACKUP_DIR/.$name.partial"
	# Grava em arquivo temporário e só renomeia no fim: um backup que falhou
	# no meio nunca aparece como válido.
	if pg_dump --format=custom --no-owner --file="$tmp"; then
		mv "$tmp" "$BACKUP_DIR/$name"
		log "gravado $name ($(du -h "$BACKUP_DIR/$name" | cut -f1))"
	else
		rm -f "$tmp"
		log "FALHOU"
		return 1
	fi
	find "$BACKUP_DIR" -maxdepth 1 -name 'payback-*.dump' -type f -mtime +"$KEEP" -print -delete |
		while read -r old; do log "apagado por retenção: $(basename "$old")"; done
}

# Segundos até a próxima ocorrência de HOUR:00, no horário local (TZ).
# Só aritmética sobre hora, minuto e segundo: funciona igual no busybox da
# imagem alpine e no date do GNU.
seconds_until_next_run() {
	set -- $(date '+%H %M %S')
	h=${1#0}; m=${2#0}; s=${3#0}
	wait=$(( (HOUR * 3600 - (h * 3600 + m * 60 + s) + 86400) % 86400 ))
	if [ "$wait" -eq 0 ]; then
		wait=86400
	fi
	echo "$wait"
}

case "${1:-loop}" in
now)
	backup_now
	;;
loop)
	log "agendado para as ${HOUR}h, mantendo ${KEEP} dias em $BACKUP_DIR"
	while true; do
		wait=$(seconds_until_next_run)
		sleep "$wait"
		# Uma falha não derruba o agendamento; tenta de novo no dia seguinte.
		backup_now || true
	done
	;;
*)
	echo "uso: backup.sh now | loop" >&2
	exit 2
	;;
esac
