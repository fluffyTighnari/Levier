# Levier — 静态站点

`site/` 下的首页与文档页都是**无构建静态文件**，不需要 npm、Node 或打包步骤。

```text
site/
  index.html                 首页
  docs/
    index.html               文档壳
    manifest.json            文档导航 + 接入点
    content/*.md             文档正文
  assets/
    css/tokens.css           色板、字体、基础层
    css/site.css             首页
    css/docs.css             文档页
    js/site.js               顶栏滚动阴影
    js/trace.js              接入流程播放器
    js/docs.js               manifest + Markdown 渲染 + callout
    vendor/marked.min.js     Markdown 解析
    vendor/purify.min.js     HTML 清洗
```

## 1. 本地预览

文档页需要读取本地 `.md`，浏览器在 `file://` 下会拦截 `fetch`。必须通过 HTTP 预览：

```powershell
cd D:\Workspace\Code\middleware
python -m http.server 4173 --directory site
```

然后打开：

```text
http://127.0.0.1:4173/
http://127.0.0.1:4173/docs/
```

首页本身也可以直接双击 `index.html` 打开，但文档页会提示需要本地 HTTP 服务。

## 2. 首页结构

首页按「价值 → 成本 → 机制 → 证据 → 边界 → 入口」推进，刻意只保留六段：

```text
Hero（一句承诺 + 一张 Run 回执）
§01 接入成本（已有函数 vs 新增声明）
§02 一次 Run，你只需要看四件事
§03 真实命令（五步可回放终端）
§04 边界与代价（负责 / 你负责 / 不包含）
§05 从哪开始（三条文档路径）
```

旧锚点保持不变，外部链接继续可用：

```text
#principe   → 接入成本
#anatomie   → 一次 Run
#integration→ 真实命令
#capacites  → 边界
```

终端回放由 `assets/js/trace.js` 驱动，命令与仓库 README 对齐；
ID、耗时与模型回复为示意，页面上有明确说明。

## 3. 新增一篇文档

只有两步。

**第一步**：从模板复制一份正文：

```text
site/docs/content/_template.md
  → site/docs/content/your-topic.md
```

**第二步**：在 `site/docs/manifest.json` 的某个 section 里追加入口：

```json
{
  "slug": "your-topic",
  "title": "你的标题",
  "status": "published",
  "file": "content/your-topic.md",
  "description": "一句话说明。"
}
```

刷新页面即可。`slug` 就是路由：

```text
docs/index.html#/your-topic
```

### 正文约定

- 文件头可有 frontmatter：`title`、`status`、`description`、`updated`。
- 标题 `##` 会自动生成锚点与右侧目录；`###` 只生成锚点，不进目录。
- 代码块自动获得语言标签与复制按钮。
- 指向其他 `.md` 的链接会自动转成站内路由。
- 支持四类 callout，写法是 GitHub 风格：

```markdown
> [!NOTE]
> 读完之后：...

> [!OUTPUT]
> 应该看到...

> [!FAIL]
> 先检查什么，再看哪条日志。

> [!TIP]
> 提示...
```

渲染后分别对应「说明 / 应该看到 / 失败时 / 提示」。
写步骤时优先给出**预期输出**与**失败时的动作**，不要只写命令。

## 4. 设计令牌

所有颜色与字体在 `assets/css/tokens.css`：

```text
--paper       #F1EBDF   暖纸
--paper-2     #E9E0D0   次级底色
--ink         #1C1915   墨
--ink-2       #3B352D   正文
--muted       #665D50   注释
--accent      #A4472C   锈红（唯一强调色）
```

字体只保留三种声音：

```text
展示    Newsreader + Noto Serif SC
正文    IBM Plex Sans + Noto Sans SC
代码    IBM Plex Mono
```

原则：

- 衬线只用于标题与少量数字，不用于正文。
- `mono` 只用于代码、命令、事件名，不再给全站标签加宽字距大写。
- 强调色只用于链接、当前状态、索引与关键标记。
- 动效只保留一次性入场与终端逐行播放；没有循环装饰动画。

## 5. 部署

把整个 `site/` 目录上传到任意静态托管即可。文档页使用 hash 路由，不需要服务端 rewrite 规则。

## 6. 与仓库 `docs/` 的关系

`site/docs/content/` 是**面向公开读者的文档**；仓库 `docs/` 中的 SCOPE、DESIGN、ADR、报告等属于内部或开发文档。
需要公开时，挑选并改写后放入 `content/`，不要直接复制全部内部计划。
