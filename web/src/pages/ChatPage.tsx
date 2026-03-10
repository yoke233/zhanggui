import { useState, useRef, useEffect } from "react";
import {
  Search,
  Send,
  Plus,
  Bot,
  User,
  MoreHorizontal,
  GitBranch,
  CheckCircle2,
  Loader2,
  ArrowRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface Session {
  id: string;
  title: string;
  preview: string;
  time: string;
  status: "active" | "idle" | "closed";
  unread?: boolean;
}

interface ChatMessage {
  id: string;
  role: "user" | "agent";
  content: string;
  time: string;
  flowCard?: {
    name: string;
    status: string;
    steps: number;
  };
}

export function ChatPage() {
  const [sessions] = useState<Session[]>([
    { id: "s1", title: "重构后端 API", preview: "好的，我已经完成了 3 个步骤的生成...", time: "2 分钟前", status: "active", unread: true },
    { id: "s2", title: "认证模块优化", preview: "分析了现有的认证流程，建议...", time: "15 分钟前", status: "active" },
    { id: "s3", title: "部署流水线配置", preview: "CI/CD 配置已更新完毕", time: "1 小时前", status: "idle" },
    { id: "s4", title: "数据库迁移方案", preview: "推荐使用增量迁移的方式...", time: "昨天", status: "closed" },
  ]);

  const [activeSession, setActiveSession] = useState("s1");
  const [sessionSearch, setSessionSearch] = useState("");
  const [messageInput, setMessageInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const [messages] = useState<ChatMessage[]>([
    {
      id: "m1",
      role: "user",
      content: "请帮我设计后端 API 的重构方案，目标是提高可维护性和测试覆盖率。",
      time: "14:20",
    },
    {
      id: "m2",
      role: "agent",
      content: "好的，我已经分析了当前 API 的代码结构。基于以下几点：\n\n1. **路由层过于耦合** — handler 直接操作数据库，没有 service 层中间 CRUD\n2. **缺少统一错误处理** — 每个 handler 各自处理错误，格式不统一\n3. **测试覆盖率低** — 大部分 handler 没有对应测试\n\n建议分为以下步骤进行重构：",
      time: "14:21",
    },
    {
      id: "m3",
      role: "agent",
      content: "我已生成了一个包含 7 个步骤的 Flow。你可以在下面查看：",
      time: "14:22",
      flowCard: {
        name: "后端 API 重构",
        status: "running",
        steps: 7,
      },
    },
    {
      id: "m4",
      role: "user",
      content: "看起来不错。测试部分能不能并行执行？另外我想加一个性能测试步骤。",
      time: "14:23",
    },
    {
      id: "m5",
      role: "agent",
      content: "当然可以。我已经做了如下调整：\n\n1. 将「编写测试」和「性能测试」设置为并行执行（它们依赖同一个上游步骤「实现 API」）\n2. 新增了「性能测试」步骤，使用 worker 角色\n3. 在集成测试的 Gate 之前加入 Repository 性能基准\n\n需要我立即执行更新后的 Flow 吗？",
      time: "14:24",
    },
    {
      id: "m6",
      role: "user",
      content: "好的，开始执行。",
      time: "14:25",
    },
    {
      id: "m7",
      role: "agent",
      content: "Flow 已启动执行。当前状态：\n\n- **需求分析** — 已完成\n- **实现 API** — 运行中\n\n我会持续跟踪进展并及时汇报。",
      time: "14:25",
    },
  ]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const filteredSessions = sessions.filter((s) =>
    s.title.toLowerCase().includes(sessionSearch.toLowerCase()),
  );

  const currentSession = sessions.find((s) => s.id === activeSession);

  return (
    <div className="flex h-full overflow-hidden">
      {/* Session list sidebar */}
      <div className="w-72 border-r flex flex-col bg-sidebar">
        <div className="border-b p-3">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold">会话列表</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7">
              <Plus className="h-4 w-4" />
            </Button>
          </div>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索会话..."
              className="h-8 pl-8 text-xs"
              value={sessionSearch}
              onChange={(e) => setSessionSearch(e.target.value)}
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {filteredSessions.map((session) => (
            <button
              key={session.id}
              onClick={() => setActiveSession(session.id)}
              className={cn(
                "w-full text-left px-3 py-3 border-b transition-colors",
                activeSession === session.id ? "bg-accent" : "hover:bg-muted/50",
              )}
            >
              <div className="flex items-center justify-between">
                <span className={cn(
                  "text-sm font-medium truncate",
                  session.unread && "font-semibold",
                )}>
                  {session.title}
                </span>
                <span className="text-[10px] text-muted-foreground shrink-0 ml-2">{session.time}</span>
              </div>
              <div className="flex items-center gap-1.5 mt-1">
                <div className={cn(
                  "h-1.5 w-1.5 rounded-full shrink-0",
                  session.status === "active" ? "bg-emerald-500" :
                  session.status === "idle" ? "bg-amber-500" : "bg-zinc-300",
                )} />
                <p className="text-xs text-muted-foreground truncate">{session.preview}</p>
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Chat main */}
      <div className="flex-1 flex flex-col">
        {/* Chat header */}
        <div className="flex items-center justify-between border-b px-5 py-3">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <Bot className="h-4 w-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold">{currentSession?.title ?? "Lead Agent"}</span>
                <Badge variant={
                  currentSession?.status === "active" ? "success" :
                  currentSession?.status === "idle" ? "warning" : "secondary"
                } className="text-[10px]">
                  {currentSession?.status === "active" ? "活跃" :
                   currentSession?.status === "idle" ? "空闲" : "已关闭"}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">Lead Agent · claude-lead · 对话中</p>
            </div>
          </div>
          <Button variant="ghost" size="icon" className="h-8 w-8">
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </div>

        {/* Messages */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          {messages.map((msg) => (
            <div key={msg.id} className={cn(
              "flex gap-3 max-w-[720px]",
              msg.role === "user" ? "ml-auto flex-row-reverse" : "",
            )}>
              <div className={cn(
                "flex h-8 w-8 shrink-0 items-center justify-center rounded-full",
                msg.role === "user" ? "bg-zinc-200" : "bg-primary text-primary-foreground",
              )}>
                {msg.role === "user" ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
              </div>
              <div className={cn(
                "space-y-2",
                msg.role === "user" ? "text-right" : "",
              )}>
                <div className={cn(
                  "rounded-lg px-4 py-3 text-sm leading-relaxed",
                  msg.role === "user"
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted",
                )}>
                  {msg.content.split("\n").map((line, i) => (
                    <span key={i}>
                      {line.startsWith("- **") ? (
                        <span className="block mt-1">
                          <strong>{line.replace(/\*\*/g, "").replace("- ", "")}</strong>
                        </span>
                      ) : line.startsWith("1.") || line.startsWith("2.") || line.startsWith("3.") ? (
                        <span className="block mt-1">{line.replace(/\*\*/g, "")}</span>
                      ) : (
                        <span className="block">{line.replace(/\*\*/g, "")}</span>
                      )}
                    </span>
                  ))}
                </div>

                {msg.flowCard && (
                  <Card className="overflow-hidden">
                    <CardContent className="p-3">
                      <div className="flex items-center gap-3">
                        <div className="flex h-9 w-9 items-center justify-center rounded-md bg-blue-50">
                          <GitBranch className="h-4 w-4 text-blue-600" />
                        </div>
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium">{msg.flowCard.name}</span>
                            <Badge variant={msg.flowCard.status === "running" ? "info" : "success"} className="text-[10px]">
                              {msg.flowCard.status === "running" ? "运行中" : "已完成"}
                            </Badge>
                          </div>
                          <span className="text-xs text-muted-foreground">{msg.flowCard.steps} 个步骤</span>
                        </div>
                        <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs">
                          查看 <ArrowRight className="h-3 w-3" />
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                )}

                <span className="text-[10px] text-muted-foreground">{msg.time}</span>
              </div>
            </div>
          ))}
          <div ref={messagesEndRef} />
        </div>

        {/* Input area */}
        <div className="border-t p-4">
          <div className="flex items-end gap-3">
            <div className="relative flex-1">
              <Input
                placeholder="输入消息，与 Lead Agent 对话..."
                className="pr-10"
                value={messageInput}
                onChange={(e) => setMessageInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    // TODO: send message
                  }
                }}
              />
            </div>
            <Button size="icon" className="h-10 w-10 shrink-0">
              <Send className="h-4 w-4" />
            </Button>
          </div>
          <div className="mt-2 flex items-center gap-4 text-[10px] text-muted-foreground">
            <span>Enter 发送 · Shift+Enter 换行</span>
            <span>Lead Agent 可以创建 Flow、管理步骤、回答问题</span>
          </div>
        </div>
      </div>
    </div>
  );
}
