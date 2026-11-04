<!-- Middleware — implementation report index. -->

# 实施报告索引

从 **P0-05** 起，每个步骤完成后必须新增一份 `docs/reports/Px-yy.md`，记录该步实际新增内容、验证结果、风险与下一步。

| 步骤 | 报告 | 状态 |
|---|---|---|
| P0-01 | `docs/ARCHIVE.md` / ADR-0001 前的归档记录 | 历史，未按本格式 |
| P0-02 | `docs/adr/0001-workflow-engine.md` | 历史，未按本格式 |
| P0-03 | `docs/adr/0002-harness.md` | 历史，未按本格式 |
| P0-04 | `docs/adr/0003-scope.md` | 历史，未按本格式 |
| P0-05 | `docs/reports/P0-05.md` | 已提交 |
| P0-06 | `docs/reports/P0-06.md` | 已提交 |
| P0-07 | `docs/reports/P0-07.md` | 已提交 |
| P1-08 | `docs/reports/P1-08.md` | 已提交 |
| P1-09 | `docs/reports/P1-09.md` | 已提交 |
| P1-10 | `docs/reports/P1-10.md` | 已提交 |
| P1-11 | `docs/reports/P1-11.md` | 已提交 |

模板：`docs/reports/TEMPLATE.md`。

**规则：**

1. 报告在代码/文档提交的同一轮完成，不能事后补写。
2. 报告必须列出实际新增/修改文件，不能只写“实现完成”。
3. 报告必须包含可复现的验证命令与结果。
4. 报告必须记录未完成项与风险；没有风险就写“无”，不能省略。
5. 下一步开始前，先确认上一份报告存在且已提交。
