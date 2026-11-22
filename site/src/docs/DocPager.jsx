import { Box, ButtonBase, Typography } from "@mui/material";
import { ArrowLeft, ArrowRight } from "@phosphor-icons/react";
import { docPresentation } from "../content/nav.en";

function PagerLink({ item, direction, locale, copy }) {
  if (!item) return <Box />;
  const presentation = docPresentation(item, locale);
  const next = direction === "next";
  return (
    <ButtonBase
      component="a"
      href={`#/${item.slug}`}
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: next ? "flex-end" : "flex-start",
        textAlign: next ? "right" : "left",
        gap: 0.5,
        p: 1.75,
        border: 1,
        borderColor: "divider",
        borderRadius: 0.75,
        backgroundColor: "background.paper",
        transition: "background-color 160ms ease, border-color 160ms ease",
        "&:hover": { backgroundColor: "action.hover", borderColor: "text.disabled" },
        "&:active": { backgroundColor: "action.selected" },
        "&:focus-visible": { outline: "2px solid", outlineColor: "primary.main", outlineOffset: 2 }
      }}
    >
      <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.75, color: "text.secondary" }}>
        {!next ? <ArrowLeft size={13} /> : null}
        <Typography component="span" sx={{ fontSize: "0.625rem", color: "text.secondary" }}>
          {next ? copy.pager.next : copy.pager.previous}
        </Typography>
        {next ? <ArrowRight size={13} /> : null}
      </Box>
      <Typography sx={{ fontFamily: (theme) => theme.fonts.display, fontSize: "0.9375rem", fontWeight: 500, lineHeight: 1.3, color: "text.primary" }}>
        {presentation.title}
      </Typography>
    </ButtonBase>
  );
}

export function DocPager({ previous, next, locale, copy }) {
  if (!previous && !next) return null;
  return (
    <Box
      component="nav"
      aria-label={copy.pager.aria}
      sx={{
        display: "grid",
        gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
        gap: 1.5,
        mt: 6,
        pt: 2.25,
        borderTop: 1,
        borderColor: "text.primary"
      }}
    >
      <PagerLink item={previous} direction="previous" locale={locale} copy={copy} />
      <PagerLink item={next} direction="next" locale={locale} copy={copy} />
    </Box>
  );
}
