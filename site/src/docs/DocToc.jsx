import { Box, ListItemButton, Typography } from "@mui/material";

function TocItem({ heading, active, slug, onNavigate }) {
  return (
    <ListItemButton
      component="a"
      href={`#/${slug}`}
      onClick={(event) => {
        event.preventDefault();
        onNavigate(heading.id);
      }}
      selected={active === heading.id}
      aria-current={active === heading.id ? "location" : undefined}
      sx={{
        display: "block",
        minHeight: 0,
        pl: heading.depth === 3 ? 2.6 : 1.25,
        pr: 0.5,
        py: 0.45,
        borderRadius: 0,
        borderLeft: 2,
        borderLeftStyle: "solid",
        borderLeftColor: active === heading.id ? "primary.main" : "transparent",
        bgcolor: "transparent",
        color: active === heading.id ? "primary.main" : "text.secondary",
        fontSize: heading.depth === 3 ? "0.6875rem" : "0.71875rem",
        lineHeight: 1.5,
        fontWeight: 400,
        boxShadow: "none",
        "&:hover": { bgcolor: "transparent", color: "text.primary" },
        "&.Mui-selected": {
          bgcolor: "transparent",
          color: "primary.main",
          fontWeight: 500,
          boxShadow: "none",
          "&:hover": { bgcolor: "transparent", color: "primary.main" }
        }
      }}
    >
      {heading.text}
    </ListItemButton>
  );
}

export function TocList({ headings, activeId, slug, onNavigate }) {
  return (
    <Box sx={{ display: "grid", gap: 0.125, mt: 1 }}>
      {headings.map((heading) => (
        <TocItem
          key={heading.id}
          heading={heading}
          active={activeId}
          slug={slug}
          onNavigate={onNavigate}
        />
      ))}
    </Box>
  );
}

export function DocToc({ headings, activeId, slug, onNavigate, copy }) {
  if (!headings || headings.length === 0) return null;
  return (
    <Box component="nav" aria-label={copy.toc.aria} sx={{ minWidth: 0 }}>
      <Typography
        component="p"
        sx={{
          pb: 1,
          borderBottom: 1,
          borderColor: "divider",
          fontFamily: (theme) => theme.fonts.mono,
          fontSize: "0.59375rem",
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: "text.secondary"
        }}
      >
        {copy.toc.title}
      </Typography>
      <TocList headings={headings} activeId={activeId} slug={slug} onNavigate={onNavigate} />
    </Box>
  );
}
