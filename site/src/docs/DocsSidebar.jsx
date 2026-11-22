import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  IconButton,
  InputAdornment,
  Link,
  ListItemButton,
  Skeleton,
  TextField,
  Typography
} from "@mui/material";
import { MagnifyingGlass, X } from "@phosphor-icons/react";
import { docPresentation, sectionLabelFor } from "../content/nav.en";

function SkeletonNav({ label }) {
  return (
    <Box sx={{ display: "grid", gap: 1.25, mt: 2.5 }} aria-label={label}>
      {Array.from({ length: 9 }).map((_, index) => (
        <Skeleton key={index} variant="text" width={`${58 + ((index * 13) % 34)}%`} height={18} />
      ))}
    </Box>
  );
}

export function DocsSidebar({
  sections,
  activeSlug,
  query,
  onQuery,
  onNavigate,
  status,
  error,
  onRetry,
  onClose,
  total,
  locale,
  copy
}) {
  const t = copy.sidebar;
  const [localQuery, setLocalQuery] = useState(query || "");
  const value = query ?? localQuery;
  const setValue = onQuery || setLocalQuery;

  const groups = useMemo(() => {
    const needle = value.trim().toLowerCase();
    return (sections || [])
      .map((section) => {
        const label = sectionLabelFor(section.title, locale);
        const items = (section.items || []).map((item) => {
          const english = docPresentation(item, "en");
          const chinese = { title: item.title, description: item.description };
          return {
            ...item,
            display: docPresentation(item, locale),
            searchText: [english.title, english.description, chinese.title, chinese.description, item.slug, label]
              .join(" ")
              .toLowerCase()
          };
        });
        const filtered = needle ? items.filter((item) => item.searchText.includes(needle)) : items;
        return { label, items: filtered };
      })
      .filter((group) => group.items.length > 0);
  }, [sections, value, locale]);

  const resultCount = groups.reduce((sum, group) => sum + group.items.length, 0);

  return (
    <Box component="nav" aria-label={t.title} sx={{ width: "100%", minWidth: 0 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
          gap: 1.5,
          pb: 1.5,
          borderBottom: 1,
          borderColor: "text.primary"
        }}
      >
        <Typography sx={{ fontSize: "0.9375rem", fontWeight: 600, color: "text.primary" }}>{t.title}</Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.625rem", color: "text.secondary", whiteSpace: "nowrap" }}>
            {status === "loading"
              ? t.loading
              : status === "error"
                ? t.unavailable
                : value.trim()
                  ? t.filtered(resultCount, total)
                  : t.pages(total)}
          </Typography>
          {onClose ? (
            <IconButton size="small" onClick={onClose} aria-label={copy.closeNavAria} sx={{ ml: 0.5 }}>
              <X size={18} />
            </IconButton>
          ) : null}
        </Box>
      </Box>

      <TextField
        fullWidth
        value={value}
        onChange={(event) => setValue(event.target.value)}
        placeholder={t.filter}
        sx={{ mt: 1.75 }}
        slotProps={{
          htmlInput: { "aria-label": t.filterAria },
          input: {
            startAdornment: (
              <InputAdornment position="start">
                <MagnifyingGlass size={15} />
              </InputAdornment>
            ),
            endAdornment: value ? (
              <InputAdornment position="end">
                <IconButton size="small" aria-label={t.clearFilter} onClick={() => setValue("")}>
                  <X size={14} />
                </IconButton>
              </InputAdornment>
            ) : null
          }
        }}
      />

      {status === "loading" ? <SkeletonNav label={t.loading} /> : null}

      {status === "error" ? (
        <Alert severity="error" sx={{ mt: 2.5, alignItems: "flex-start" }}>
          <Typography sx={{ fontSize: "0.75rem", fontWeight: 600 }}>{t.errorTitle}</Typography>
          <Typography sx={{ mt: 0.5, fontSize: "0.6875rem", lineHeight: 1.6, color: "text.secondary" }}>
            {error || "manifest.json"}
          </Typography>
          <Button variant="outlined" size="small" onClick={onRetry} sx={{ mt: 1.25, minHeight: 28, px: 1.25 }}>
            {t.retry}
          </Button>
        </Alert>
      ) : null}

      {status === "empty" ? (
        <Box sx={{ mt: 2.5, p: 2, border: 1, borderStyle: "dashed", borderColor: "divider", borderRadius: 1, textAlign: "center" }}>
          <Typography sx={{ fontSize: "0.75rem", fontWeight: 600, color: "text.primary" }}>{t.emptyTitle}</Typography>
          <Typography sx={{ mt: 0.5, fontSize: "0.6875rem", lineHeight: 1.6, color: "text.secondary" }}>
            {t.emptyBody}
          </Typography>
        </Box>
      ) : null}

      {status === "ready" && value.trim() && groups.length === 0 ? (
        <Box sx={{ mt: 2.5, p: 2.5, border: 1, borderStyle: "dashed", borderColor: "divider", borderRadius: 1, textAlign: "center" }}>
          <Typography sx={{ fontSize: "0.75rem", fontWeight: 600, color: "text.primary" }}>{t.noResultsTitle}</Typography>
          <Typography sx={{ mt: 0.5, fontSize: "0.6875rem", lineHeight: 1.6, color: "text.secondary" }}>
            {t.noResultsBody}
          </Typography>
        </Box>
      ) : null}

      {status === "ready" && groups.length > 0
        ? groups.map((group) => (
            <Box key={group.label} sx={{ mt: 2.75 }}>
              <Typography
                component="p"
                sx={{
                  mb: 0.5,
                  fontFamily: (theme) => theme.fonts.mono,
                  fontSize: "0.59375rem",
                  letterSpacing: "0.07em",
                  textTransform: "uppercase",
                  color: "text.secondary"
                }}
              >
                {group.label}
              </Typography>
              <Box sx={{ display: "grid", gap: 0.125 }}>
                {group.items.map((item) => (
                  <ListItemButton
                    key={item.slug}
                    component="a"
                    href={`#/${item.slug}`}
                    onClick={onNavigate}
                    selected={activeSlug === item.slug}
                    aria-current={activeSlug === item.slug ? "page" : undefined}
                    sx={{
                      mx: -1.25,
                      minHeight: 30,
                      py: 0.5,
                      fontSize: "0.78125rem",
                      lineHeight: 1.45,
                      "& .MuiTypography-root": {
                        fontSize: "0.78125rem",
                        fontWeight: activeSlug === item.slug ? 600 : 400
                      }
                    }}
                  >
                    <Typography>{item.display.title}</Typography>
                  </ListItemButton>
                ))}
              </Box>
            </Box>
          ))
        : null}

      <Box sx={{ mt: 3, pt: 2, borderTop: 1, borderColor: "divider" }}>
        <Typography sx={{ fontSize: "0.6875rem", lineHeight: 1.7, color: "text.secondary" }}>
          {t.footer}
          <Link href="#/errors" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>
            {t.troubleshooting}
          </Link>
          {t.footerTail}
        </Typography>
        <Typography sx={{ mt: 1.25, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.59375rem", lineHeight: 1.6, color: "text.secondary" }}>
          {t.languageNote}
        </Typography>
      </Box>
    </Box>
  );
}
