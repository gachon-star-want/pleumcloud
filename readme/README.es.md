<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">Todo tu almacenamiento en la nube gratuito, en un solo disco.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="Licencia: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · Español · [Français](README.fr.md) · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud es una aplicación local-first que convierte tu almacenamiento gratuito disperso — 15 GB de Google, 30 GB de Naver MyBox, 5 GB de OneDrive, 20 GB de Drime, 10 GB de pCloud… más de 100 GB repartidos en seis apps — en **un solo disco**. Conecta tus cuentas una vez y luego explora, busca, sube y mueve archivos como si vivieran todos en un único disco. Cada archivo lleva una insignia que muestra en qué nube vive.

Los archivos nunca se rehospedan: cada archivo vive íntegro en una nube, y PleumCloud solo mantiene un índice local más tus credenciales en el llavero del sistema. Se distribuye como un único binario de ~18 MB con la interfaz incrustada.

## Funciones

- 🔗 **Conexión en un clic** — OAuth oficial para Google Drive, OneDrive, Dropbox y pCloud; tokens de acceso para Naver MyBox, Drime, Koofr y MediaFire; WebDAV para el resto. Las credenciales viven en el llavero del sistema
- 🗂️ **Un disco unificado** — explora todas las nubes en una sola vista con insignias de nube por archivo, migas de pan y un panel de cuotas por nube en tiempo real
- 🔍 **Búsqueda instantánea en todas partes** — una sola pulsación busca en todas las cuentas conectadas mediante un índice local de texto completo (SQLite FTS5)
- 🧠 **Colocación inteligente** — las subidas van donde tengas más espacio libre. Anúlalo por subida, o define reglas (*"vídeos → Google, PDFs → MyBox, más de 1 GB → pCloud"*) en el editor de reglas
- 🚚 **Transferencias entre nubes** — copia o mueve archivos entre nubes como trabajos en segundo plano que fluyen a través de tu máquina. Cierra la pestaña; la transferencia sigue
- 👁 **Vistas previas y streaming** — imágenes, vídeo con posicionamiento, audio y PDF, más una vista de galería con miniaturas generadas localmente
- 🔗 **Enlaces para compartir, descargas, renombrar/mover/copiar/eliminar** — las operaciones de archivo de cada día, desde un solo sitio
- 🖥️ **Local-first** — un solo binario, sin cuentas, sin telemetría. ¿Necesitas acceso remoto? `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` lo protege con autenticación Basic (combínalo con HTTPS en un NAS/VPS)
- 🧩 **18 proveedores** — 10 conectores nativos hechos a mano más 8 a través del puente rclone, todos tras una única interfaz

## Nubes compatibles

| Proveedor | Nivel gratuito | Conexión | Estado |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ Nativo |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ Nativo |
| Dropbox | 2 GB | OAuth 2.0 | ✅ Nativo |
| Naver MyBox | 30 GB | Token de acceso | ✅ Nativo |
| Drime | 20 GB | Token de acceso | ✅ Nativo |
| pCloud | 10 GB | OAuth 2.0 | ✅ Nativo |
| Koofr | 10 GB | Correo + token | ✅ Nativo |
| WebDAV (Nextcloud, ownCloud, MagentaCloud, …) | — | URL + usuario | ✅ Nativo |
| InfiniCLOUD | 20 GB | URL + usuario | ✅ Nativo |
| MediaFire | 10 GB | Credenciales de app | ✅ Nativo |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | Puente [rclone](https://rclone.org) | 🧪 Experimental |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ Sin API oficial — [motivos](../docs/decisions.md) |

La matriz completa de todos los servicios evaluados, y el razonamiento detrás de las decisiones de diseño principales, vive en [docs/decisions.md](../docs/decisions.md).

## Instalación

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

Después ejecuta `pleumcloud` — tu navegador se abre en `http://localhost:7777`.

**Otras formas:**

```bash
# desde el código fuente (Go 1.26+ y Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: descarga el .zip desde Releases
```

¿Primera conexión OAuth? Consigue una clave de app gratuita una vez siguiendo [docs/oauth-setup.md](../docs/oauth-setup.md) (10 minutos, una vez por máquina) — después, cada conexión es un solo clic.

## Cómo funciona

```
        ┌──────────────────────────────────────────────┐
        │                pleumcloud                    │
        │  ┌──────────┐  ┌─────────┐  ┌─────────────┐  │
 You ──▶│  │  Web UI  │  │ Indexer │  │  Placement  │  │
        │  └────┬─────┘  └────┬────┘  └──────┬──────┘  │
        │  ┌────┴────────────┴───────────────┴──────┐  │
        │        provider connectors × 18           │  │
        │  └───┬──────┬──────┬──────┬──────┬───────┘  │
        └──────┼──────┼──────┼──────┼──────┼───────────┘
               ▼      ▼      ▼      ▼      ▼
           Google   One   Dropbox  MyBox   WebDAV …
           (files stay on YOUR clouds — PleumCloud is the control plane)
```

PleumCloud es el plano de control, no un host. El indexador mantiene un catálogo local SQLite/FTS5 de nombres, tamaños y fechas (sincronizado incrementalmente vía la fuente de cambios de cada proveedor), el motor de colocación decide a dónde van los archivos nuevos, y las transferencias entre nubes fluyen origen → tu máquina → destino usando el protocolo de subida reanudable de cada proveedor.

## Desarrollo

Requiere Go 1.26+ y Node 20+.

```bash
make dev    # go run + servidor dev de Vite (hot reload)
make build  # bundle del frontend → binario Go incrustado
make test   # go test ./...
```

El backend vive en `internal/` (api, provider, index, placement, …) y la app React en `web/`.

## Contribuir

Los issues y PRs son bienvenidos — incluidos **nuevos conectores**, mejoras de UI, docs y traducciones. Cada conector vive en `internal/provider/<name>/` tras una única interfaz con tests de mocks; empieza por [CONTRIBUTING.md](../CONTRIBUTING.md).

## Privacidad

Sin telemetría, sin analítica, sin cuentas. Los tokens se quedan en el llavero del sistema; el índice se queda en `~/.pleumcloud/`. Detalles: [política de privacidad](../web/public/privacy.html).

## Licencia

PleumCloud es software libre bajo la **GNU AGPL-3.0** — úsalo, estúdialo, modifícalo y haz self-hosting libremente. Si lo ofreces como servicio de red, debes compartir tus modificaciones bajo la misma licencia.
