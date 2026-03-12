import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Link2, Loader2, Send, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useWorkbench } from "@/contexts/WorkbenchContext";
import { formatRelativeTime, getErrorMessage } from "@/lib/v2Workbench";
import { Link } from "react-router-dom";
import type { Thread, ThreadMessage, ThreadParticipant, ThreadWorkItemLink, Issue } from "@/types/apiV2";

export function ThreadDetailPage() {
  const { t } = useTranslation();
  const { threadId } = useParams<{ threadId: string }>();
  const navigate = useNavigate();
  const { apiClient } = useWorkbench();

  const [thread, setThread] = useState<Thread | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [participants, setParticipants] = useState<ThreadParticipant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [workItemLinks, setWorkItemLinks] = useState<ThreadWorkItemLink[]>([]);
  const [linkedIssues, setLinkedIssues] = useState<Record<number, Issue>>({});
  const [newMessage, setNewMessage] = useState("");
  const [sending, setSending] = useState(false);

  const id = Number(threadId);

  useEffect(() => {
    if (!id || isNaN(id)) return;
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [th, msgs, parts, links] = await Promise.all([
          apiClient.getThread(id),
          apiClient.listThreadMessages(id, { limit: 100 }),
          apiClient.listThreadParticipants(id),
          apiClient.listWorkItemsByThread(id),
        ]);
        if (!cancelled) {
          setThread(th);
          setMessages(msgs);
          setParticipants(parts);
          setWorkItemLinks(links);
          // Fetch issue details for each link.
          const issueMap: Record<number, Issue> = {};
          const issueResults = await Promise.allSettled(
            links.map((l) => apiClient.getIssue(l.work_item_id)),
          );
          issueResults.forEach((r, i) => {
            if (r.status === "fulfilled") issueMap[links[i].work_item_id] = r.value;
          });
          if (!cancelled) setLinkedIssues(issueMap);
        }
      } catch (e) {
        if (!cancelled) setError(getErrorMessage(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [apiClient, id]);

  const handleSend = async () => {
    if (!newMessage.trim() || !id) return;
    setSending(true);
    try {
      const msg = await apiClient.createThreadMessage(id, {
        content: newMessage.trim(),
        role: "human",
      });
      setMessages((prev) => [...prev, msg]);
      setNewMessage("");
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setSending(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error || !thread) {
    return (
      <div className="space-y-4 p-6">
        <Button variant="ghost" size="sm" onClick={() => navigate("/threads")}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          {t("threads.backToList", "Back to Threads")}
        </Button>
        <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error || t("threads.notFound", "Thread not found")}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col space-y-4 p-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" onClick={() => navigate("/threads")}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-xl font-bold">{thread.title}</h1>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Badge variant={thread.status === "active" ? "default" : "secondary"}>
              {thread.status}
            </Badge>
            {thread.owner_id && <span>{t("threads.owner", "Owner")}: {thread.owner_id}</span>}
            <span>{formatRelativeTime(thread.updated_at)}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-1 gap-4 overflow-hidden">
        {/* Messages area */}
        <Card className="flex flex-1 flex-col">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">
              {t("threads.messages", "Messages")} ({messages.length})
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-1 flex-col overflow-hidden">
            <div className="flex-1 space-y-3 overflow-y-auto pb-4">
              {messages.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {t("threads.noMessages", "No messages yet. Start the conversation.")}
                </p>
              ) : (
                messages.map((msg) => (
                  <div
                    key={msg.id}
                    className={`rounded-lg px-3 py-2 text-sm ${
                      msg.role === "agent"
                        ? "bg-muted"
                        : "bg-primary/5"
                    }`}
                  >
                    <div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
                      <Badge variant="outline" className="text-[10px]">
                        {msg.role}
                      </Badge>
                      <span>{msg.sender_id || "anonymous"}</span>
                      <span>{formatRelativeTime(msg.created_at)}</span>
                    </div>
                    <p className="whitespace-pre-wrap">{msg.content}</p>
                  </div>
                ))
              )}
            </div>

            {/* Send input */}
            <div className="flex gap-2 border-t pt-3">
              <Input
                placeholder={t("threads.messagePlaceholder", "Type a message...")}
                value={newMessage}
                onChange={(e) => setNewMessage(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && !e.shiftKey && handleSend()}
                disabled={sending || thread.status !== "active"}
              />
              <Button
                size="sm"
                onClick={handleSend}
                disabled={!newMessage.trim() || sending || thread.status !== "active"}
              >
                <Send className="h-4 w-4" />
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* Participants panel */}
        <Card className="w-60 shrink-0">
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Users className="h-4 w-4" />
              {t("threads.participants", "Participants")} ({participants.length})
            </CardTitle>
          </CardHeader>
          <CardContent>
            {participants.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {t("threads.noParticipants", "No participants")}
              </p>
            ) : (
              <div className="space-y-2">
                {participants.map((p) => (
                  <div key={p.id} className="flex items-center gap-2 text-sm">
                    <Badge variant="outline" className="text-[10px]">
                      {p.role}
                    </Badge>
                    <span className="truncate">{p.user_id}</span>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Linked Work Items */}
      {workItemLinks.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Link2 className="h-4 w-4" />
              {t("threads.linkedWorkItems", "Linked Work Items")} ({workItemLinks.length})
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {workItemLinks.map((link) => {
                const issue = linkedIssues[link.work_item_id];
                return (
                  <div key={link.id} className="flex items-center gap-2 text-sm">
                    {link.is_primary && (
                      <Badge variant="default" className="text-[10px]">
                        {t("threads.primary", "primary")}
                      </Badge>
                    )}
                    <Badge variant="outline" className="text-[10px]">
                      {link.relation_type}
                    </Badge>
                    <Link
                      to={`/issues/${link.work_item_id}`}
                      className="font-medium text-primary hover:underline"
                    >
                      {issue ? issue.title : `#${link.work_item_id}`}
                    </Link>
                    {issue && (
                      <Badge variant="secondary" className="text-[10px]">
                        {issue.status}
                      </Badge>
                    )}
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      )}

      {thread.summary && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">{t("threads.summary", "Summary")}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">{thread.summary}</p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
