# OAuth setup guide

PleumCloud connects to Google Drive, OneDrive, Dropbox and pCloud with
one-click OAuth: the provider's own sign-in page opens and PleumCloud never
asks for your cloud account password.

## Which app key gets used

The OAuth client is resolved in this order:

1. **Your own app** (BYO) — pasted once per machine if you've set one; it
   wins over everything. See the per-provider guides below.
2. **The project's official app** — compiled into the binary
   (`internal/oauthflow/defaults.go`). When present, connects are one
   click with zero setup.
3. Server/self-host deployments can override either level with
   `PLEUMCLOUD_OAUTH_<PROVIDER>_CLIENT_ID` / `_CLIENT_SECRET`
   (e.g. `PLEUMCLOUD_OAUTH_GDRIVE_CLIENT_ID=… pleumcloud`).

The official apps ship as **public client IDs** (rclone-style): a
local-first binary cannot keep a secret confidential anyway, every flow
uses PKCE (S256), and each app is locked to its registered redirect URIs.
The same guides below serve both users pasting their own key and the
project registering an official app.

## Google Drive (15 GB free)

1. Open the [Google Cloud Console](https://console.cloud.google.com/) and
   create a project.
2. **APIs & Services → Library** → enable **Google Drive API**.
3. **APIs & Services → OAuth consent screen**:
   - User type: **External**, app name `PleumCloud`, your support email.
   - Add the scope `https://www.googleapis.com/auth/drive`.
   - Add your own Google account as a **test user** (testing mode caps
     usage at 100 users until verification).
4. **APIs & Services → Credentials → Create credentials → OAuth client ID**:
   - Type: **Web application**
   - Authorized redirect URIs:
     - `http://localhost:7777/api/connect/gdrive/callback`
     - `http://127.0.0.1:7777/api/connect/gdrive/callback`
5. Copy the Client ID and Secret into PleumCloud: **Connect a cloud →
   Google Drive → one-time setup**.

## Microsoft OneDrive (5 GB free)

1. [Azure Portal → Microsoft Entra ID → App registrations → New
   registration](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade).
2. Name `PleumCloud`; **Supported account types: personal Microsoft
   accounts only** (consumers); redirect URI (Web):
   `http://localhost:7777/api/connect/onedrive/callback`.
3. **Certificates & secrets → New client secret** → copy the value.
4. No admin consent needed for personal accounts.

## Dropbox (2 GB free)

1. [Dropbox App Console](https://www.dropbox.com/developers/apps) →
   **Create app**.
2. API: scoped access; type: Full Dropbox; name `PleumCloud`.
3. **Permissions** tab: enable `files.content.write`,
   `files.content.read`, `files.metadata.read`, `account_info.read`.
4. **Settings → OAuth 2 → Redirect URIs**:
   `http://localhost:7777/api/connect/dropbox/callback`.
5. Development-status apps cap at 50 linked users; production approval
   (free) lifts it when needed.

## pCloud (10 GB free)

1. [pCloud My Apps](https://docs.pcloud.com/my_apps.html) → register an
   app (Full access).
2. Redirect URI: `http://localhost:7777/api/connect/pcloud/callback`.

## Token storage

Tokens live in your OS keychain (macOS Keychain, Windows Credential
Manager, libsecret on Linux; a 0600 file on headless systems). They never
leave your machine in local mode.

## Using a different port?

If you run `PLEUMCLOUD_PORT=9000`, register
`http://localhost:9000/api/connect/<provider>/callback` instead.
