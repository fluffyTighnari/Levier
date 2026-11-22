import { useEffect, useMemo } from "react";
import { Box } from "@mui/material";
import { renderMarkdown } from "../lib/markdown";
import { DocHeader } from "./DocHeader";
import { DocPager } from "./DocPager";
import { MobileToc } from "./MobileToc";

export function DocArticle({
  item,
  meta,
  body,
  onHeadings,
  previous,
  next,
  activeHeading,
  onTocNavigate,
  tocExpanded,
  onTocChange,
  showMobileToc,
  slug,
  locale,
  copy
}) {
  const rendered = useMemo(
    () => renderMarkdown(body, { slug, locale, copy }),
    [body, slug, locale, copy]
  );

  useEffect(() => {
    onHeadings(rendered.headings);
  }, [rendered.headings, onHeadings]);

  return (
    <Box component="article" sx={{ maxWidth: 760, minWidth: 0 }}>
      <DocHeader item={item} meta={meta} locale={locale} copy={copy} />

      {showMobileToc ? (
        <MobileToc
          headings={rendered.headings}
          activeId={activeHeading}
          slug={slug}
          onNavigate={onTocNavigate}
          expanded={tocExpanded}
          onChange={onTocChange}
          copy={copy}
        />
      ) : null}

      <Box
        lang={locale === "en" ? "en" : "zh-CN"}
        sx={{
          "& > *:first-of-type": { mt: 0 },
          "& p": { color: "text.secondary" },
          "& strong": { color: "text.primary" }
        }}
      >
        {rendered.node}
      </Box>

      <DocPager previous={previous} next={next} locale={locale} copy={copy} />
    </Box>
  );
}
