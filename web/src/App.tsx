import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import Sidebar from "./components/Sidebar";
import TopBar from "./components/TopBar";
import EmptyState from "./components/EmptyState";
import ConnectPanel from "./components/ConnectPanel";
import FileBrowser from "./components/FileBrowser";
import SearchResults from "./components/SearchResults";

type View = "drive" | "connect";

export default function App() {
  const [view, setView] = useState<View>("drive");
  const [query, setQuery] = useState("");
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const connected = accounts.data?.accounts ?? [];

  const searching = query.trim().length > 0;

  return (
    <div className="flex h-full">
      <Sidebar
        connectedCount={connected.length}
        onConnect={() => setView("connect")}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar
          view={view}
          query={query}
          onQuery={setQuery}
          onHome={() => {
            setView("drive");
            setQuery("");
          }}
          onConnect={() => setView("connect")}
        />
        <main className="flex-1 overflow-y-auto p-4 sm:p-6">
          {view === "connect" ? (
            <ConnectPanel />
          ) : searching ? (
            <SearchResults query={query.trim()} />
          ) : connected.length > 0 ? (
            <FileBrowser onConnect={() => setView("connect")} />
          ) : (
            <EmptyState hasAccounts={false} onConnect={() => setView("connect")} />
          )}
        </main>
      </div>
    </div>
  );
}
