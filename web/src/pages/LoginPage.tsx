import { useState, type FormEvent } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface LoginPageProps {
  onLogin: (token: string) => void;
  loading?: boolean;
  error?: string | null;
}

export const LoginPage = ({ onLogin, loading, error }: LoginPageProps) => {
  const [token, setToken] = useState("");

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = token.trim();
    if (trimmed.length > 0) {
      onLogin(trimmed);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
      <div className="w-full max-w-sm">
        <section className="rounded-2xl border border-slate-200 bg-white p-8 shadow-[0_24px_80px_rgba(15,23,42,0.08)]">
          <Badge variant="secondary">AI Workflow</Badge>
          <h1 className="mt-4 text-2xl font-semibold tracking-tight text-slate-900">
            Login
          </h1>
          <p className="mt-1 text-sm text-slate-500">
            Enter your access token to continue.
          </p>

          <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-4">
            <Input
              type="password"
              placeholder="Access Token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoFocus
              disabled={loading}
            />

            {error ? (
              <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                {error}
              </p>
            ) : null}

            <Button type="submit" disabled={loading || token.trim().length === 0}>
              {loading ? "Verifying..." : "Login"}
            </Button>
          </form>

          <p className="mt-4 text-xs text-slate-400">
            Token is printed in the server console at startup.
          </p>
        </section>
      </div>
    </main>
  );
};
