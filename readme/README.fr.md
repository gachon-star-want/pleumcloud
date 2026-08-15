<p align="center">
  <img src="../docs/banner.svg" alt="PleumCloud" width="640">
</p>

<h3 align="center">Tout votre stockage cloud gratuit, un seul disque.</h3>

<p align="center">
  <a href="https://github.com/gachon-star-want/pleumcloud/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/gachon-star-want/pleumcloud/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square&logo=gnu&logoColor=white" alt="Licence : AGPL-3.0"></a>
</p>

<p align="center">
  🌍 [English](../README.md) · [한국어](README.ko.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [Español](README.es.md) · Français · [Deutsch](README.de.md) · [Português (Brasil)](README.pt-BR.md)
</p>

---

PleumCloud est une application local-first qui transforme votre stockage gratuit éparpillé — 15 Go de Google, 30 Go de Naver MyBox, 5 Go de OneDrive, 20 Go de Drime, 10 Go de pCloud… plus de 100 Go répartis sur six applis — en **un seul disque**. Connectez vos comptes une fois, puis parcourez, recherchez, téléversez et déplacez des fichiers comme s'ils vivaient tous sur un même disque. Chaque fichier porte un badge indiquant sur quel cloud il se trouve.

Les fichiers ne sont jamais réhébergés : chaque fichier reste entier sur un cloud, et PleumCloud ne conserve qu'un index local plus vos identifiants dans le trousseau du système. Il se distribue comme un unique binaire d'environ 18 Mo avec l'interface intégrée.

## Fonctionnalités

- 🔗 **Connexion en un clic** — OAuth officiel pour Google Drive, OneDrive, Dropbox et pCloud ; jetons d'accès pour Naver MyBox, Drime, Koofr et MediaFire ; WebDAV pour le reste. Les identifiants vivent dans votre trousseau système
- 🗂️ **Un disque unifié** — parcourez tous les clouds dans une vue unique avec badges de cloud par fichier, fil d'Ariane et tableau de bord des quotas par cloud en temps réel
- 🔍 **Recherche instantanée partout** — une seule frappe recherche dans tous les comptes connectés via un index local en texte intégral (SQLite FTS5)
- 🧠 **Placement intelligent** — les téléversements partent là où vous avez le plus d'espace libre. Forcez au cas par cas, ou définissez des règles (*« vidéos → Google, PDF → MyBox, plus de 1 Go → pCloud »*) dans l'éditeur de règles
- 🚚 **Transferts inter-clouds** — copiez ou déplacez des fichiers entre clouds comme des tâches de fond qui transitent par votre machine. Fermez l'onglet ; le transfert continue
- 👁 **Aperçus et streaming** — images, vidéo avec navigation, audio et PDF, plus une vue galerie avec vignettes générées localement
- 🔗 **Liens de partage, téléchargements, renommer/déplacer/copier/supprimer** — les opérations quotidiennes, au même endroit
- 🖥️ **Local-first** — un seul binaire, sans compte, sans télémétrie. Un accès distant ? `PLEUMCLOUD_SERVER=1 PLEUMCLOUD_PASSWORD=… pleumcloud` le protège par authentification Basic (associez-le à HTTPS sur un NAS/VPS)
- 🧩 **18 fournisseurs** — 10 connecteurs natifs façonnés à la main plus 8 via le pont rclone, tous derrière une seule interface

## Clouds pris en charge

| Fournisseur | Offre gratuite | Connexion | Statut |
|---|---|---|---|
| Google Drive | 15 Go | OAuth 2.0 | ✅ Natif |
| Microsoft OneDrive | 5 Go | OAuth 2.0 | ✅ Natif |
| Dropbox | 2 Go | OAuth 2.0 | ✅ Natif |
| Naver MyBox | 30 Go | Jeton d'accès | ✅ Natif |
| Drime | 20 Go | Jeton d'accès | ✅ Natif |
| pCloud | 10 Go | OAuth 2.0 | ✅ Natif |
| Koofr | 10 Go | E-mail + jeton | ✅ Natif |
| WebDAV (Nextcloud, ownCloud, MagentaCloud, …) | — | URL + identifiants | ✅ Natif |
| InfiniCLOUD | 20 Go | URL + identifiants | ✅ Natif |
| MediaFire | 10 Go | Identifiants d'app | ✅ Natif |
| MEGA · Box · Yandex Disk · HiDrive · Jottacloud · Filen · Internxt · Proton Drive | 5–20 Go | Pont [rclone](https://rclone.org) | 🧪 Expérimental |
| iCloud Drive · Samsung Cloud · TeraBox · Sync.com | — | — | ❌ Pas d'API officielle — [pourquoi](../docs/decisions.md) |

La matrice complète de tous les services évalués, et le raisonnement derrière les grandes décisions de conception, vit dans [docs/decisions.md](../docs/decisions.md).

## Installation

**macOS / Linux :**

```bash
curl -fsSL https://raw.githubusercontent.com/gachon-star-want/pleumcloud/main/scripts/install.sh | bash
```

Lancez ensuite `pleumcloud` — votre navigateur s'ouvre sur `http://localhost:7777`.

**Autres méthodes :**

```bash
# depuis les sources (Go 1.26+ et Node 20+)
git clone https://github.com/gachon-star-want/pleumcloud
cd pleumcloud && make build && ./pleumcloud

# Windows : téléchargez le .zip depuis les Releases
```

**Première connexion OAuth ?** Procurez-vous une clé d'app gratuite une fois via [docs/oauth-setup.md](../docs/oauth-setup.md) (10 minutes, une fois par machine) — ensuite, chaque connexion se fait en un clic.

## Fonctionnement

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

PleumCloud est le plan de contrôle, pas l'hébergeur. L'indexeur maintient un catalogue local SQLite/FTS5 des noms, tailles et dates (synchronisé de façon incrémentale via le flux de changements de chaque fournisseur), le moteur de placement décide où vont les nouveaux fichiers, et les transferts inter-clouds streament source → votre machine → destination via le protocole d'upload reprenable de chaque fournisseur.

## Développement

Nécessite Go 1.26+ et Node 20+.

```bash
make dev    # go run + serveur dev Vite (hot reload)
make build  # bundle du frontend → binaire Go embarqué
make test   # go test ./...
```

Le backend vit dans `internal/` (api, provider, index, placement, …) et l'appli React dans `web/`.

## Contribuer

Les issues et PR sont les bienvenus — y compris **de nouveaux connecteurs**, de l'UI, de la doc et des traductions. Chaque connecteur vit dans `internal/provider/<name>/` derrière une interface unique avec des tests sur mocks ; commencez par [CONTRIBUTING.md](../CONTRIBUTING.md).

## Confidentialité

Pas de télémétrie, pas d'analytics, pas de comptes. Les jetons restent dans votre trousseau système ; l'index reste dans `~/.pleumcloud/`. Détails : [politique de confidentialité](../web/public/privacy.html).

## Licence

PleumCloud est un logiciel libre sous **GNU AGPL-3.0** — utilisez-le, étudiez-le, modifiez-le et auto-hébergez-le librement. Si vous l'offrez comme service en ligne, vous devez partager vos modifications sous la même licence.
