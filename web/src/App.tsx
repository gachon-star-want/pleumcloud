import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import Sidebar from "./components/Sidebar";
import TopBar from "./components/TopBar";
import EmptyState from "./components/EmptyState";
import ConnectPanel from "./components/ConnectPanel";

type View = "drive" | "connect";

export default function App() {
  const [view, setView] = useState<View>("drive");
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const connected = accounts.data?.accounts ?? [];

  return (
    <div className="flex h-full">
      <Sidebar
        connectedCount={connected.length}
        onConnect={() => setView("connect")}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar view={view} onHome={() => setView("drive")} />
        <main className="flex-1 overflow-y-auto p-6">
          {view === "connect" ? (
            <ConnectPanel />
          ) : (
            <EmptyState
              hasAccounts={connected.length > 0}
              onConnect={() => setView("connect")}
            />
          )}
        </main>
      </div>
    </div>
  );
}
