<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">Todo o seu armazenamento em nuvem gratuito, em um só disco.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="Licença: AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · Português (Brasil)
</p>

---

O PleumCloud é um aplicativo local-first que transforma seu armazenamento gratuito espalhado — 15 GB do Google, 30 GB do Naver MyBox, 5 GB do OneDrive, 20 GB do Drime, 10 GB do pCloud… mais de 100 GB distribuídos em seis apps — em **um só disco**. Conecte suas contas uma vez e depois navegue, pesquise, envie e mova arquivos como se todos vivessem num único disco. Cada arquivo carrega um selo mostrando em qual nuvem ele vive.

Os arquivos nunca são re-hospedados: cada arquivo vive inteiro em uma nuvem, e o PleumCloud mantém apenas um índice local mais suas credenciais no chaveiro do sistema. Ele é distribuído como um único binário de ~18 MB com a interface embutida.

## Funcionalidades

- 🔗 **Conexão com um clique** — OAuth oficial para Google Drive, OneDrive, Dropbox e pCloud; tokens de acesso para Naver MyBox, Drime, Koofr e MediaFire; WebDAV para o resto. As credenciais ficam no chaveiro do sistema
- 🗂️ **Um disco unificado** — navegue por todas as nuvens numa única visão, com selos de nuvem por arquivo, trilha de navegação e um painel de cotas por nuvem em tempo real
- 🔍 **Busca instantânea em toda parte** — uma única tecla busca em todas as contas conectadas através de um índice local de texto completo (SQLite FTS5)
- 🧠 **Colocação inteligente** — os envios vão para onde você tem mais espaço livre. Sobrescreva por envio, ou defina regras (*"vídeos → Google, PDFs → MyBox, acima de 1 GB → pCloud"*) no editor de regras
- 🚚 **Transferências entre nuvens** — copie ou mova arquivos entre nuvens como tarefas em segundo plano que fluem pela sua máquina. Feche a aba; a transferência continua
- 👁 **Prévias e streaming** — imagens, vídeo com seek, áudio e PDF, além de uma visão de galeria com miniaturas geradas localmente
- 🔗 **Links de compartilhamento, downloads, renomear/mover/copiar/excluir** — as operações de arquivo do dia a dia, num só lugar
- 🖥️ **Local-first** — um único binário, sem conta, sem telemetria. Precisa de acesso remoto? `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` o protege com autenticação Basic (combine com HTTPS num NAS/VPS)
- 🧩 **18 provedores** — 10 conectores nativos feitos à mão mais 8 através da ponte rclone, todos atrás de uma única interface

## Nuvens compatíveis

| Provedor | Nível gratuito | Conexão | Status |
|---|---|---|---|
| Google Drive | 15 GB | OAuth 2.0 | ✅ Nativo |
| Microsoft OneDrive | 5 GB | OAuth 2.0 | ✅ Nativo |
| Dropbox | 2 GB | OAuth 2.0 | ✅ Nativo |
| Naver MyBox | 30 GB | Token de acesso | ✅ Nativo |
| Drime | 20 GB | Token de acesso | ✅ Nativo |
| pCloud | 10 GB | OAuth 2.0 | ✅ Nativo |
| Koofr | 10 GB | E-mail + token | ✅ Nativo |
| WebDAV (Nextcloud, ownCloud, MagentaCloud, …) | — | URL + login | ✅ Nativo |
| InfiniCLOUD | 20 GB | URL + login | ✅ Nativo |
| MediaFire | 10 GB | Credenciais de app | ✅ Nativo |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 GB | Ponte [rclone](https://rclone.org) | 🧪 Experimental |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ Sem API oficial — [motivo](../docs/decisions.md) |

A matriz completa de todos os serviços avaliados, e o raciocínio por trás das principais decisões de design, vive em [docs/decisions.md](../docs/decisions.md).

## Instalação

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

Depois execute `pleumcloud` — seu navegador abre em `http://localhost:7777`.

**Outras formas:**

```bash
# a partir do código-fonte (Go 1.26+ e Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows: baixe o .zip dos Releases
```

**Primeira conexão OAuth?** Consiga uma chave de app gratuita uma vez seguindo [docs/oauth-setup.md](../docs/oauth-setup.md) (10 minutos, uma vez por máquina) — depois disso, cada conexão é um clique.

## Como funciona

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

O PleumCloud é o plano de controle, não o host. O indexador mantém um catálogo local SQLite/FTS5 de nomes, tamanhos e datas (sincronizado incrementalmente via o feed de mudanças de cada provedor), o motor de colocação decide para onde vão os novos arquivos, e as transferências entre nuvens fazem streaming de origem → sua máquina → destino usando o protocolo de upload retomável de cada provedor.

## Desenvolvimento

Requer Go 1.26+ e Node 20+.

```bash
make dev    # go run + servidor dev do Vite (hot reload)
make build  # bundle do frontend → binário Go embutido
make test   # go test ./...
```

O backend vive em `internal/` (api, provider, index, placement, …) e o app React em `web/`.

## Contribuindo

Issues e PRs são bem-vindos — incluindo **novos conectores**, melhorias de UI, docs e traduções. Cada conector vive em `internal/provider/<name>/` atrás de uma única interface com testes de mock; comece pelo [CONTRIBUTING.md](../CONTRIBUTING.md).

## Privacidade

Sem telemetria, sem analytics, sem contas. Os tokens ficam no chaveiro do sistema; o índice fica em `~/.pleumcloud/`. Detalhes: [política de privacidade](../web/public/privacy.html).

## Licença

O PleumCloud é software livre sob a **GNU AGPL-3.0** — use, estude, modifique e faça self-hosting livremente. Se você o oferecer como serviço de rede, deve compartilhar suas modificações sob a mesma licença.
