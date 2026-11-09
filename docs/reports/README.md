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
| P1-12 | `docs/reports/P1-12.md` | 已提交 |
| P1-13 | `docs/reports/P1-13.md` | 已提交（G1 待确认） |
| P2-14 | `docs/reports/P2-14.md` | 已提交 |
| P2-15 | `docs/reports/P2-15.md` | 已提交 |
| P2-16 | `docs/reports/P2-16.md` | 已提交 |
| P2-17 | `docs/reports/P2-17.md` | 已提交 |
| P2-18 | `docs/reports/P2-18.md` | 已提交 |
| P2-19 | `docs/reports/P2-19.md` | 已提交 |
| P2-20 | `docs/reports/P2-20.md` | 已提交 |
| P2-21 | `docs/reports/P2-21.md` | 已提交 |
| P2-22 | `docs/reports/P2-22.md` | 已提交（G2 通过） |
| P3-23 | `docs/reports/P3-23.md` | 已提交 |
| P3-24 | `docs/reports/P3-24.md` | 已提交 |
| P3-25 | `docs/reports/P3-25.md` | 已提交 |
| P3-26 | `docs/reports/P3-26.md` | 已提交 |
| P3-27 | `docs/reports/P3-27.md` | 已提交 |
| P3-28 | `docs/reports/P3-28.md` | 已提交 |
| P3-29 | `docs/reports/P3-29.md` | 已提交 |
| P3-30 | `docs/reports/P3-30.md` | 已提交 |
| G3 | `docs/reports/G3.md` | 已提交 |
| P4-31 | `docs/reports/P4-31.md` | 已提交 |
| P4-32 | `docs/reports/P4-32.md` | 已提交 |
| P4-33 | `docs/reports/P4-33.md` | 已提交 |
| P4-34 | `docs/reports/P4-34.md` | 已提交 |
| P4-35 | `docs/reports/P4-35.md` | 已提交 |
| P4-36 | `docs/reports/P4-36.md` | 已提交 |
| P5-37 | `docs/reports/P5-37.md` | 已提交 |
| P5-38 | `docs/reports/P5-38.md` | 已提交 |
| P5-39 | `docs/reports/P5-39.md` | 已提交 |
| P5-40 | `docs/reports/P5-40.md` | 已提交 |
| P5-41 | `docs/reports/P5-41.md` | 已提交 |
| P5-42 | `docs/reports/P5-42.md` | 已提交 |
| G5 | `docs/reports/G5.md` | 已提交 |
| P6-43 | `docs/reports/P6-43.md` | 已提交 |
| P6-44 | `docs/reports/P6-44.md` | 已提交 |
| P6-45 | `docs/reports/P6-45.md` | 已提交 |
| P6-46 | `docs/reports/P6-46.md` | 已提交 |
| G4 | `docs/reports/G4.md` | 已提交 |

模板：`docs/reports/TEMPLATE.md`。

## 专题报告

| 报告 | 内容 |
|---|---|
| `docs/reports/WORKFLOW_TRACE.md` | Workflow 层全量逐函数追踪、输入输出、竞态与测试说明 |
| `docs/reports/BOUNDARY_DATA_FLOW.md` | Middleware/Daemon 数据结构与边界流：Envelope、Task、事件、工具、重连 |

**规则：**

1. 报告在代码/文档提交的同一轮完成，不能事后补写。
2. 报告必须列出实际新增/修改文件，不能只写“实现完成”。
3. 报告必须包含可复现的验证命令与结果。
4. 报告必须记录未完成项与风险；没有风险就写“无”，不能省略。
5. 下一步开始前，先确认上一份报告存在且已提交。
