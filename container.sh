#!/usr/bin/env bash
# ==============================================================================
# Unified Container Management Helper for AI Sales Agent Workshop (Go)
# Automatically detects and uses Docker or Podman
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ------------------------------------------------------------------------------
# 1. Detect Container Engine & Compose Tool
# ------------------------------------------------------------------------------
detect_tools() {
    if command -v podman-compose >/dev/null 2>&1; then
        COMPOSE_CMD="podman-compose"
        ENGINE_CMD="podman"
    elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
        COMPOSE_CMD="podman compose"
        ENGINE_CMD="podman"
    elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
        ENGINE_CMD="docker"
    elif command -v docker-compose >/dev/null 2>&1; then
        COMPOSE_CMD="docker-compose"
        ENGINE_CMD="docker"
    elif command -v podman >/dev/null 2>&1; then
        COMPOSE_CMD="podman"
        ENGINE_CMD="podman"
    elif command -v docker >/dev/null 2>&1; then
        COMPOSE_CMD="docker"
        ENGINE_CMD="docker"
    else
        echo "❌ Error: Tidak ditemukan 'docker' maupun 'podman' di sistem Anda."
        echo "   Silakan instal Docker atau Podman terlebih dahulu."
        exit 1
    fi
}

detect_tools

# ------------------------------------------------------------------------------
# 2. Print Header & Prerequisite Checks
# ------------------------------------------------------------------------------
print_banner() {
    echo "=================================================================="
    echo "  Glowria AI Sales Agent (Go) — Container Management"
    echo "  Engine  : $ENGINE_CMD"
    echo "  Compose : $COMPOSE_CMD"
    echo "=================================================================="
}

ensure_env_file() {
    if [ ! -f "$SCRIPT_DIR/.env" ]; then
        echo "⚠️  File .env tidak ditemukan. Menyalin dari .env.example..."
        cp "$SCRIPT_DIR/.env.example" "$SCRIPT_DIR/.env"
    fi
}

ensure_data_dirs() {
    mkdir -p "$SCRIPT_DIR/data" "$SCRIPT_DIR/media"
    ensure_env_file
}

# ------------------------------------------------------------------------------
# 3. Command Handlers
# ------------------------------------------------------------------------------
cmd_help() {
    print_banner
    echo "Penggunaan: ./container.sh <perintah>"
    echo ""
    echo "Perintah yang Tersedia:"
    echo "  build        Build container image (ai-sales-agent-go:latest)"
    echo "  up           Jalankan Sales Agent saja (Go Server, port 8000)"
    echo "  up:full      Jalankan Sales Agent + Evolution API lokal (port 8083)"
    echo "  up:postgres  Jalankan Sales Agent + PostgreSQL DB lokal (port 5439)"
    echo "  up:all       Jalankan SEMUA service (Sales Agent + Evolution API + PostgreSQL)"
    echo "  down         Hentikan semua container dan jaringan"
    echo "  logs         Pantau log real-time dari container Sales Agent"
    echo "  cli          Buka simulator chat WhatsApp interaktif di dalam container"
    echo "  status       Periksa status dan healthcheck container"
    echo "  help         Tampilkan pesan bantuan ini"
    echo ""
}

cmd_build() {
    print_banner
    echo "🔨 Membangun image ai-sales-agent-go:latest..."
    $ENGINE_CMD build -t ai-sales-agent-go:latest -f Dockerfile .
    echo "✅ Build selesai!"
}

cmd_up() {
    print_banner
    ensure_data_dirs
    echo "🚀 Menjalankan Glowria Sales Agent (default, tanpa Postgres & Evolution)..."
    $COMPOSE_CMD up -d app
    echo ""
    echo "✅ Aplikasi berjalan!"
    echo "   - Webhook Root        : http://localhost:8000/"
    echo "   - Admin Dashboard     : http://localhost:8000/dashboard"
    echo "   - Cek log             : ./container.sh logs"
    echo "   - Chat CLI interaktif : ./container.sh cli"
}

cmd_up_full() {
    print_banner
    ensure_data_dirs
    echo "🚀 Menjalankan Sales Agent + Evolution API lokal..."
    if [ "$COMPOSE_CMD" = "podman-compose" ]; then
        COMPOSE_PROFILES=full $COMPOSE_CMD up -d
    else
        $COMPOSE_CMD --profile full up -d
    fi
    echo ""
    echo "✅ Seluruh service berjalan!"
    echo "   - Sales Agent Dashboard : http://localhost:8000/dashboard"
    echo "   - Evolution WhatsApp UI : http://localhost:8083/"
}

cmd_up_postgres() {
    print_banner
    ensure_data_dirs
    echo "🚀 Menjalankan Sales Agent + PostgreSQL..."
    if [ "$COMPOSE_CMD" = "podman-compose" ]; then
        COMPOSE_PROFILES=postgres $COMPOSE_CMD up -d
    else
        $COMPOSE_CMD --profile postgres up -d
    fi
    echo ""
    echo "✅ Service PostgreSQL dan Sales Agent berjalan!"
    echo "   - Sales Agent Dashboard : http://localhost:8000/dashboard"
    echo "   - PostgreSQL host port  : localhost:5439"
}

cmd_up_all() {
    print_banner
    ensure_data_dirs
    echo "🚀 Menjalankan SEMUA service (Sales Agent + Evolution API + PostgreSQL)..."
    if [ "$COMPOSE_CMD" = "podman-compose" ]; then
        COMPOSE_PROFILES=full,postgres $COMPOSE_CMD up -d
    else
        $COMPOSE_CMD --profile full --profile postgres up -d
    fi
    echo ""
    echo "✅ Semua service berjalan!"
    echo "   - Sales Agent Dashboard : http://localhost:8000/dashboard"
    echo "   - Evolution WhatsApp UI : http://localhost:8083/"
    echo "   - PostgreSQL host port  : localhost:5439"
}

cmd_down() {
    print_banner
    echo "🛑 Menghentikan seluruh container..."
    $COMPOSE_CMD down
    echo "✅ Seluruh container telah dihentikan."
}

cmd_logs() {
    $COMPOSE_CMD logs -f app
}

cmd_cli() {
    print_banner
    echo "💬 Menghubungkan ke simulator WhatsApp di dalam container..."
    # Pastikan container app sedang menyala
    if ! $ENGINE_CMD ps --format '{{.Names}}' | grep -q "glowria-sales-agent"; then
        echo "⚠️ Container belum berjalan. Menyalakan container terlebih dahulu..."
        cmd_up
    fi
    $ENGINE_CMD exec -it glowria-sales-agent /app/cli
}

cmd_status() {
    print_banner
    $COMPOSE_CMD ps
}

# ------------------------------------------------------------------------------
# 4. Entrypoint Router
# ------------------------------------------------------------------------------
ACTION="${1:-help}"

case "$ACTION" in
    build)
        cmd_build
        ;;
    up)
        cmd_up
        ;;
    up:full)
        cmd_up_full
        ;;
    up:postgres)
        cmd_up_postgres
        ;;
    up:all)
        cmd_up_all
        ;;
    down)
        cmd_down
        ;;
    logs)
        cmd_logs
        ;;
    cli)
        cmd_cli
        ;;
    status)
        cmd_status
        ;;
    help|--help|-h)
        cmd_help
        ;;
    *)
        echo "❌ Perintah tidak dikenal: $ACTION"
        echo ""
        cmd_help
        exit 1
        ;;
esac
