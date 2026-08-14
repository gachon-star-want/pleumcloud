package provider

// Catalog is the static provider matrix surfaced in the UI's
// "connect a cloud" screen. Connector implementations register themselves
// into the live registry; the catalog describes what we *offer*, including
// providers whose connectors arrive in later milestones.
//
// Free tier sizes verified 2026-08 via the provider research report.
var Catalog = []Metadata{
	{ID: "gdrive", Name: "Google Drive", AuthKind: AuthOAuth2, Tier: TierNative, FreeTierGB: 15,
		DocsURL: "https://developers.google.com/drive"},
	{ID: "onedrive", Name: "Microsoft OneDrive", AuthKind: AuthOAuth2, Tier: TierNative, FreeTierGB: 5,
		DocsURL: "https://learn.microsoft.com/onedrive/developer/rest-api/"},
	{ID: "dropbox", Name: "Dropbox", AuthKind: AuthOAuth2, Tier: TierNative, FreeTierGB: 2,
		DocsURL: "https://www.dropbox.com/developers/documentation"},
	{ID: "mybox", Name: "Naver MyBox", AuthKind: AuthPAT, Tier: TierNative, FreeTierGB: 30,
		DocsURL: "https://developers.mybox.naver.com"},
	{ID: "drime", Name: "Drime", AuthKind: AuthPAT, Tier: TierNative, FreeTierGB: 20,
		DocsURL: "https://docs.drime.cloud"},
	{ID: "pcloud", Name: "pCloud", AuthKind: AuthOAuth2, Tier: TierNative, FreeTierGB: 10,
		DocsURL: "https://docs.pcloud.com"},
	{ID: "koofr", Name: "Koofr", AuthKind: AuthPAT, Tier: TierNative, FreeTierGB: 10,
		DocsURL: "https://app.koofr.net/developers/api"},
	{ID: "webdav", Name: "WebDAV", AuthKind: AuthWebDAV, Tier: TierNative, FreeTierGB: 0,
		DocsURL: "https://en.wikipedia.org/wiki/WebDAV"},

	// Experimental tier — served through the rclone sidecar bridge (M2+).
	{ID: "mega", Name: "MEGA", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 20},
	{ID: "box", Name: "Box", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 10},
	{ID: "mediafire", Name: "MediaFire", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 10,
		DocsURL: "https://www.mediafire.com/developers/",
		// 10 GB base, expandable to ~50 GB via bonuses; official REST API
		// with app registration. Added after the namu.wiki list review (2026-08).
		},
	{ID: "yandex", Name: "Yandex Disk", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 5},
	{ID: "hidrive", Name: "STRATO HiDrive", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 0},
	{ID: "jottacloud", Name: "Jottacloud", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 5},
	{ID: "filen", Name: "Filen", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 10},
	{ID: "internxt", Name: "Internxt", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 10},
	{ID: "protondrive", Name: "Proton Drive", AuthKind: AuthBridge, Tier: TierExperimental, FreeTierGB: 5},
	{ID: "infinicloud", Name: "InfiniCLOUD", AuthKind: AuthWebDAV, Tier: TierNative, FreeTierGB: 20,
		DocsURL: "https://infini-cloud.net/en/developer_api.html"},
}
