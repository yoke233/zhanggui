import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { ApiClient } from "@/lib/apiClient";
import type { Project } from "@/types/workflow";
import CommandCenterView from "@/views/CommandCenterView";
import { PageScaffold } from "@/v3/components/PageScaffold";

interface OverviewViewProps {
  apiClient: ApiClient;
  projectId: string | null;
  projects: Project[];
  selectedProject: Project | null;
  refreshToken: number;
  onNavigate: (view: "chat" | "board" | "runs" | "ops") => void;
}

const OverviewView = ({
  apiClient,
  projectId,
  projects,
  selectedProject,
  refreshToken,
  onNavigate,
}: OverviewViewProps) => {
  if (!projectId) {
    return (
      <PageScaffold
        eyebrow="Command Center"
        title="总览指挥台"
        description="今天需要你处理的不是所有事，而是最关键的 8 件事。"
        contextTitle="尚未选择项目"
        contextMeta="先在运维页创建项目，再回到总览查看关键进展。"
        stats={[
          { label: "当前项目", value: "0", helper: "还没有可进入的工作区" },
          { label: "下一步", value: "Create", helper: "先创建项目，再加载 Issue / Run / Session" },
          { label: "入口", value: "Ops", helper: "项目初始化和审计入口已经收口到运维页" },
        ]}
        actions={[
          {
            label: "前往协议 / 运维",
            onClick: () => onNavigate("ops"),
          },
        ]}
      >
        <Card className="border-dashed border-slate-300 bg-white/80">
          <CardHeader>
            <CardTitle>当前没有可展示的业务总览</CardTitle>
            <CardDescription>
              v3 总览依赖项目上下文。请先在“协议 / 运维”页面创建或导入项目，然后刷新项目列表。
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-3 text-sm text-slate-600">
            <span>已发现项目数：{projects.length}</span>
            <Button variant="outline" onClick={() => onNavigate("ops")}>
              打开项目创建入口
            </Button>
          </CardContent>
        </Card>
      </PageScaffold>
    );
  }

  return (
    <CommandCenterView
      apiClient={apiClient}
      projectId={projectId}
      projects={projects}
      selectedProject={selectedProject}
      refreshToken={refreshToken}
      onNavigate={onNavigate}
    />
  );
};

export default OverviewView;
