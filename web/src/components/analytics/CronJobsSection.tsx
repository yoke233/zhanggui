import { useTranslation } from "react-i18next";
import { CalendarClock, Pause, Play, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Link } from "react-router-dom";
import { parseCronExpr } from "@/lib/cronParser";
import { formatRelativeTime } from "@/lib/v2Workbench";
import type { CronStatus } from "@/types/apiV2";

interface Props {
  cronIssues: CronStatus[];
  onToggle: (cron: CronStatus) => Promise<void>;
  onAdd: () => void;
}

export function CronJobsSection({ cronIssues, onToggle, onAdd }: Props) {
  const { t } = useTranslation();

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2">
            <CalendarClock className="h-4 w-4 text-indigo-500" />
            {t("analytics.cronJobs")}
          </CardTitle>
          <Button variant="outline" size="sm" onClick={onAdd}>
            <Plus className="mr-1.5 h-3.5 w-3.5" />
            {t("analytics.addCronJob")}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("analytics.flowId")}</TableHead>
              <TableHead>{t("analytics.cronExpression")}</TableHead>
              <TableHead>{t("analytics.status")}</TableHead>
              <TableHead>{t("analytics.maxConcurrent")}</TableHead>
              <TableHead>{t("analytics.lastTriggered")}</TableHead>
              <TableHead>{t("analytics.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cronIssues.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground">
                  {t("analytics.noCronJobs")}
                </TableCell>
              </TableRow>
            ) : (
              cronIssues.map((c) => (
                <TableRow key={c.work_item_id}>
                  <TableCell>
                    <Link to={`/work-items/${c.work_item_id}`} className="text-blue-600 hover:underline">
                      #{c.work_item_id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <span className="font-mono text-xs">{c.schedule}</span>
                    {c.schedule ? (
                      <span className="ml-2 text-xs text-muted-foreground">
                        {parseCronExpr(c.schedule, t).description ?? ""}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Badge variant={c.enabled ? "success" : "secondary"} className="text-xs">
                      {c.enabled ? t("analytics.cronEnabled") : t("analytics.cronDisabled")}
                    </Badge>
                  </TableCell>
                  <TableCell>{c.max_instances ?? 1}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {c.last_triggered ? formatRelativeTime(c.last_triggered) : t("analytics.neverTriggered")}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 px-2"
                      onClick={() => void onToggle(c)}
                    >
                      {c.enabled ? (
                        <>
                          <Pause className="mr-1 h-3 w-3" />
                          {t("analytics.disable")}
                        </>
                      ) : (
                        <>
                          <Play className="mr-1 h-3 w-3" />
                          {t("analytics.enable")}
                        </>
                      )}
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
