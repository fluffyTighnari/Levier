import { Box, Breadcrumbs, Link, Typography } from "@mui/material";
import { docPresentation, sectionLabelFor } from "../content/nav.en";

export function DocHeader({ item, meta, locale, copy }) {
  const base = docPresentation(item, locale);
  const title = meta.title || base.title;
  const description = meta.description || base.description;
  const section = sectionLabelFor(item.section, locale);
  const status = meta.status || item.status || "published";
  const statusLabel =
    status === "published" ? copy.docHeader.published : status === "draft" ? copy.docHeader.draft : status;

  return (
    <Box component="header" sx={{ pb: 2.5, mb: 3.5, borderBottom: 1, borderColor: "text.primary" }}>
      <Breadcrumbs aria-label={copy.docHeader.breadcrumbAria} sx={{ mb: 2 }}>
        <Link href="#/onboarding" underline="hover" sx={{ color: "text.secondary", fontSize: "0.75rem" }}>
          {copy.docHeader.documentation}
        </Link>
        <Typography sx={{ fontSize: "0.75rem", color: "text.secondary" }}>{section}</Typography>
      </Breadcrumbs>

      <Typography
        component="h1"
        variant="h1"
        sx={{ fontSize: { xs: "1.95rem", md: "2.6rem" }, lineHeight: 1.12, textWrap: "balance" }}
      >
        {title}
      </Typography>

      {description ? (
        <Typography sx={{ maxWidth: "56ch", mt: 1.5, fontSize: { xs: "0.9375rem", md: "1rem" }, lineHeight: 1.7, color: "text.secondary" }}>
          {description}
        </Typography>
      ) : null}

      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: 0.75,
          mt: 2,
          fontFamily: (theme) => theme.fonts.mono,
          fontSize: "0.625rem",
          color: "text.secondary"
        }}
      >
        {meta.updated ? <span>{copy.docHeader.updated} {meta.updated}</span> : null}
        <Box component="span" aria-hidden="true">·</Box>
        <span>{statusLabel}</span>
        <Box component="span" aria-hidden="true">·</Box>
        <span>{copy.docHeader.language}</span>
      </Box>
    </Box>
  );
}
