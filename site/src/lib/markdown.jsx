import {
  Alert,
  AlertTitle,
  Box,
  Divider,
  Link,
  List,
  ListItem,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography
} from "@mui/material";
import { alpha } from "@mui/material/styles";
import { Info, Lightbulb, TerminalWindow, Warning } from "@phosphor-icons/react";
import { marked } from "marked";
import { CopyButton } from "../components/CopyButton";

const CALLOUTS = {
  NOTE: { severity: "info", icon: Info },
  TIP: { severity: "warning", icon: Lightbulb },
  IMPORTANT: { severity: "info", icon: Info },
  WARNING: { severity: "warning", icon: Warning },
  CAUTION: { severity: "error", icon: Warning },
  OUTPUT: { severity: "success", icon: TerminalWindow },
  FAIL: { severity: "error", icon: Warning }
};

const SEVERITY_COLOR = {
  info: "info",
  success: "success",
  warning: "warning",
  error: "error"
};

export function parseFrontmatter(raw) {
  const text = String(raw || "").replace(/^\uFEFF/, "");
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(text);
  if (!match) return { meta: {}, body: text };

  const meta = {};
  for (const line of match[1].split(/\r?\n/)) {
    const pair = /^([A-Za-z0-9_-]+)\s*:\s*(.*)$/.exec(line);
    if (pair) meta[pair[1]] = pair[2].trim().replace(/^["']|["']$/g, "");
  }
  return { meta, body: text.slice(match[0].length) };
}

function slugify(text, used) {
  let base = String(text || "")
    .trim()
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^\p{L}\p{N}\-]+/gu, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
  base = base || "section";
  let id = base;
  let n = 2;
  while (used[id]) {
    id = `${base}-${n}`;
    n += 1;
  }
  used[id] = true;
  return id;
}

export function plainText(tokens) {
  if (!tokens) return "";
  return tokens
    .map((token) => {
      if (typeof token === "string") return token;
      if (token.text) return token.text;
      if (token.tokens) return plainText(token.tokens);
      return "";
    })
    .join("");
}

function stripMarker(tokens) {
  let stripped = false;
  return (tokens || [])
    .map((token) => {
      if (!stripped && token.type === "text") {
        stripped = true;
        return {
          ...token,
          raw: token.raw.replace(/^\[![A-Z]+\]\s*/, ""),
          text: token.text.replace(/^\[![A-Z]+\]\s*/, "")
        };
      }
      if (token.tokens) return { ...token, tokens: stripMarker(token.tokens) };
      return token;
    })
    .filter((token) => !(token.type === "text" && !token.text.trim()));
}

function prepareCallout(block) {
  const match = /^\[!([A-Z]+)\]/.exec(block.text || "");
  if (!match) return null;
  const kind = match[1];
  const tokens = (block.tokens || [])
    .map((token, index) => {
      if (index !== 0 || token.type !== "paragraph") return token;
      return {
        ...token,
        raw: token.raw.replace(/^\[![A-Z]+\]\s*/, ""),
        text: token.text.replace(/^\[![A-Z]+\]\s*/, ""),
        tokens: stripMarker(token.tokens)
      };
    })
    .filter((token) => !(token.type === "paragraph" && !token.text.trim()));
  return { kind, tokens };
}

function normaliseHref(href) {
  const value = String(href || "");
  if (/^https?:\/\//i.test(value)) {
    return { href: value, external: true };
  }
  const md = /^(?:.*\/)?([^/#]+)\.md(?:#([^#]*))?$/i.exec(value);
  if (md) {
    return { href: `#/${md[1]}`, external: false };
  }
  return { href: value, external: false };
}

function Inline({ tokens, ctx }) {
  if (!tokens) return null;
  return tokens.map((token, index) => {
    const key = `inline-${index}`;
    switch (token.type) {
      case "text":
        return token.tokens && token.tokens.length ? <Inline key={key} tokens={token.tokens} ctx={ctx} /> : token.text;
      case "escape":
        return token.text;
      case "strong":
        return <Box component="strong" key={key} sx={{ color: "text.primary", fontWeight: 600 }}><Inline tokens={token.tokens} ctx={ctx} /></Box>;
      case "em":
        return <Box component="em" key={key} sx={{ fontStyle: "italic" }}><Inline tokens={token.tokens} ctx={ctx} /></Box>;
      case "del":
        return <Box component="del" key={key}><Inline tokens={token.tokens} ctx={ctx} /></Box>;
      case "codespan":
        return (
          <Box
            component="code"
            key={key}
            sx={{
              px: 0.6,
              py: 0.15,
              borderRadius: 0.5,
              bgcolor: "action.hover",
              fontFamily: (theme) => theme.fonts.mono,
              fontSize: "0.86em",
              color: "text.primary",
              wordBreak: "break-word"
            }}
          >
            {token.text}
          </Box>
        );
      case "br":
        return <Box component="br" key={key} />;
      case "link": {
        const target = normaliseHref(token.href);
        return (
          <Link
            key={key}
            href={target.href}
            target={target.external ? "_blank" : undefined}
            rel={target.external ? "noopener noreferrer" : undefined}
            underline="always"
            sx={{ color: "text.primary", textDecorationColor: "primary.main" }}
          >
            <Inline tokens={token.tokens} ctx={ctx} />
            {target.external ? <Box component="span" sx={{ position: "absolute", clip: "rect(0 0 0 0)", width: "1px", height: "1px", overflow: "hidden" }}> {ctx.copy.markdown.newTab}</Box> : null}
          </Link>
        );
      }
      case "image":
        return (
          <Box
            component="img"
            key={key}
            src={token.href}
            alt={token.text || ""}
            sx={{ display: "block", maxWidth: "100%", my: 2, border: 1, borderColor: "divider", borderRadius: 1 }}
          />
        );
      case "html":
        return token.text;
      default:
        return token.text || null;
    }
  });
}

function CodeBlock({ code, language, copy }) {
  return (
    <Paper
      variant="outlined"
      sx={{ my: 2.25, borderColor: "divider", borderRadius: 1.25, overflow: "hidden", bgcolor: "background.paper" }}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          minHeight: 42,
          px: 2,
          py: 0.5,
          bgcolor: "action.hover",
          borderBottom: 1,
          borderColor: "divider"
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: "primary.main", flex: "none" }} />
          <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.71875rem", color: "text.secondary" }}>
            {language || "text"}
          </Typography>
        </Box>
        <CopyButton text={code} labels={copy.markdown} />
      </Box>
      <Box
        component="pre"
        tabIndex={0}
        aria-label={copy.markdown.codeBlock}
        sx={{
          m: 0,
          p: { xs: 1.75, md: 2.25 },
          overflowX: "auto",
          fontFamily: (theme) => theme.fonts.mono,
          fontSize: { xs: "0.71875rem", md: "0.78125rem" },
          lineHeight: 1.8,
          color: "text.primary",
          bgcolor: "background.paper"
        }}
      >
        <Box component="code" sx={{ font: "inherit" }}>{code}</Box>
      </Box>
    </Paper>
  );
}

function PlainCode({ code, copy }) {
  return (
    <Box
      component="pre"
      tabIndex={0}
      aria-label={copy.markdown.codeSample}
      sx={{
        m: "10px 0 0",
        p: 1.5,
        overflowX: "auto",
        border: 1,
        borderColor: "divider",
        borderRadius: 0.75,
        bgcolor: "background.paper",
        color: "text.secondary",
        fontFamily: (theme) => theme.fonts.mono,
        fontSize: "0.71875rem",
        lineHeight: 1.8,
        whiteSpace: "pre"
      }}
    >
      {code}
    </Box>
  );
}

function Callout({ kind, tokens, ctx }) {
  const config = CALLOUTS[kind] || CALLOUTS.NOTE;
  const Icon = config.icon;
  const severity = config.severity;
  const colorKey = SEVERITY_COLOR[severity] || "info";

  return (
    <Alert
      severity={severity}
      icon={<Icon size={18} weight="regular" />}
      sx={{
        my: 2.25,
        borderLeft: 3,
        borderLeftStyle: "solid",
        borderLeftColor: `${colorKey}.main`,
        bgcolor: (theme) => alpha(theme.palette[colorKey].main, 0.06),
        "& .MuiAlert-icon": { color: `${colorKey}.main` },
        "& .MuiAlert-message": { minWidth: 0 }
      }}
    >
      <AlertTitle sx={{ fontSize: "0.71875rem", fontWeight: 600, mb: 0.25, color: `${colorKey}.main` }}>
        {ctx.copy.markdown.callouts[kind] || kind}
      </AlertTitle>
      {renderBlocks(tokens, { ...ctx, inCallout: true })}
    </Alert>
  );
}

function MarkdownList({ ordered, start, items, ctx }) {
  return (
    <List dense disablePadding component={ordered ? "ol" : "ul"} sx={{ my: 1.5, listStyle: "none" }}>
      {items.map((item, index) => {
        const marker = ordered ? `${(start || 1) + index}.` : null;
        const nested = item.tokens.filter((token) => token.type === "list");
        const body = item.tokens.filter((token) => token.type !== "list");
        return (
          <ListItem key={`list-${index}`} disableGutters sx={{ display: "block", py: 0.4, alignItems: "flex-start" }}>
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
              {ordered ? (
                <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.75rem", color: "text.secondary", pt: 0.35, minWidth: 18, flex: "none" }}>
                  {marker}
                </Typography>
              ) : (
                <Box sx={{ width: 4, height: 4, bgcolor: "primary.main", flex: "none", mt: "0.68em" }} />
              )}
              <Box sx={{ minWidth: 0, "& > p:first-of-type": { mt: 0 } }}>
                {renderBlocks(body, ctx)}
              </Box>
            </Box>
            {nested.map((token, nestedIndex) => (
              <Box key={`nested-${nestedIndex}`} sx={{ pl: 2.5 }}>
                <MarkdownList ordered={token.ordered} start={token.start} items={token.items} ctx={ctx} />
              </Box>
            ))}
          </ListItem>
        );
      })}
    </List>
  );
}

function MarkdownTable({ token, ctx }) {
  const tableNumber = (ctx.tableCount = (ctx.tableCount || 0) + 1);
  return (
    <TableContainer
      component={Box}
      role="region"
      aria-label={ctx.copy.markdown.table(tableNumber)}
      tabIndex={0}
      sx={{
        my: 2.25,
        border: 1,
        borderColor: "divider",
        borderRadius: 0.75,
        overflowX: "auto",
        bgcolor: "background.paper",
        "&:focus-visible": { outline: "2px solid", outlineColor: "primary.main", outlineOffset: 2 }
      }}
    >
      <Table size="small" sx={{ minWidth: 420 }}>
        <TableHead>
          <TableRow>
            {token.header.map((cell, index) => (
              <TableCell key={`th-${index}`} align={token.align?.[index] || "left"}>
                <Inline tokens={cell.tokens} ctx={ctx} />
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {token.rows.map((row, rowIndex) => (
            <TableRow key={`tr-${rowIndex}`}>
              {row.map((cell, cellIndex) => (
                <TableCell key={`td-${rowIndex}-${cellIndex}`} align={token.align?.[cellIndex] || "left"}>
                  <Inline tokens={cell.tokens} ctx={ctx} />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

function renderBlocks(tokens, ctx) {
  if (!tokens) return null;
  return tokens.map((token, index) => {
    const key = `block-${index}`;
    switch (token.type) {
      case "space":
        return null;
      case "heading": {
        const text = plainText(token.tokens);
        const id = slugify(text, ctx.used);
        if (token.depth === 2 || token.depth === 3) {
          ctx.headings.push({ id, text, depth: token.depth });
        }
        if (token.depth === 2) {
          return (
            <Typography
              key={key}
              id={id}
              variant="h2"
              sx={{
                mt: 5.5,
                pt: 2.5,
                borderTop: 1,
                borderColor: "divider",
                fontSize: { xs: "1.3rem", md: "1.55rem" },
                scrollMarginTop: "90px",
                "&:first-of-type": { mt: 0, pt: 0, borderTop: 0 }
              }}
            >
              {text}
            </Typography>
          );
        }
        if (token.depth === 3) {
          return (
            <Typography
              key={key}
              id={id}
              component="h3"
              variant="h4"
              sx={{ mt: 3.5, fontSize: "1.0625rem", fontWeight: 600, scrollMarginTop: "90px" }}
            >
              {text}
            </Typography>
          );
        }
        return (
          <Typography
            key={key}
            id={id}
            component={token.depth >= 4 ? `h${Math.min(token.depth, 6)}` : "h4"}
            variant="h5"
            sx={{ mt: 2.75, fontSize: "0.9375rem", fontWeight: 600, scrollMarginTop: "90px" }}
          >
            {text}
          </Typography>
        );
      }
      case "paragraph":
        return (
          <Typography key={key} component="p" variant="body1" sx={{ mt: 1.75, "&:first-of-type": { mt: 0 } }}>
            <Inline tokens={token.tokens} ctx={ctx} />
          </Typography>
        );
      case "text":
        return (
          <Typography key={key} component="p" variant="body1" sx={{ mt: 1.75 }}>
            <Inline tokens={token.tokens} ctx={ctx} />
          </Typography>
        );
      case "code":
        return ctx.inCallout ? (
          <PlainCode key={key} code={token.text} copy={ctx.copy} />
        ) : (
          <CodeBlock key={key} code={token.text} language={token.lang} copy={ctx.copy} />
        );
      case "blockquote": {
        const callout = prepareCallout(token);
        if (callout) {
          return <Callout key={key} kind={callout.kind} tokens={callout.tokens} ctx={ctx} />;
        }
        return (
          <Box
            key={key}
            component="blockquote"
            sx={{ my: 2, pl: 2, borderLeft: 3, borderColor: "divider", color: "text.secondary" }}
          >
            {renderBlocks(token.tokens, ctx)}
          </Box>
        );
      }
      case "list":
        return <MarkdownList key={key} ordered={token.ordered} start={token.start} items={token.items} ctx={ctx} />;
      case "table":
        return <MarkdownTable key={key} token={token} ctx={ctx} />;
      case "hr":
        return <Divider key={key} sx={{ my: 4 }} />;
      case "html":
        return (
          <Typography key={key} component="p" variant="body1" sx={{ mt: 1.75, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.8125rem" }}>
            {token.text}
          </Typography>
        );
      default:
        if (token.tokens) return <Box key={key}>{renderBlocks(token.tokens, ctx)}</Box>;
        return null;
    }
  });
}

export function renderMarkdown(body, { slug, locale = "en", copy }) {
  const used = Object.create(null);
  const headings = [];
  const tokens = marked.lexer(body || "", { gfm: true });
  const ctx = { slug, locale, copy, headings, used, inCallout: false, tableCount: 0 };
  return {
    node: <>{renderBlocks(tokens, ctx)}</>,
    headings
  };
}
