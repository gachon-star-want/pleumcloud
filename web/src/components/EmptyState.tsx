import ProviderLogo from "./ProviderLogo";

interface EmptyStateProps {
  hasAccounts: boolean;
  onConnect: () => void;
}

export default function EmptyState({ hasAccounts, onConnect }: EmptyStateProps) {
  return (
    <div className="grid h-full place-items-center">
      <div className="max-w-md text-center">
        <div className="mb-6 flex items-center justify-center gap-3">
          {["gdrive", "mybox", "dropbox", "drime"].map((id) => (
            <ProviderLogo key={id} id={id} className="h-10" />
          ))}
        </div>
        <h2 className="mb-2 text-2xl font-bold">
          One drive for all your free cloud storage
        </h2>
        <p className="mb-8 text-slate-500">
          Connect Google Drive, Naver MyBox, Dropbox and more — then browse,
          search and move files as if they were on a single drive.
        </p>
        {hasAccounts ? (
          <p className="text-sm text-slate-400">
            Your files will appear here once indexing finishes.
          </p>
        ) : (
          <button
            onClick={onConnect}
            className="rounded-full bg-blue-600 px-6 py-2.5 text-sm font-semibold text-white shadow hover:bg-blue-700"
          >
            Connect your first cloud
          </button>
        )}
      </div>
    </div>
  );
}
