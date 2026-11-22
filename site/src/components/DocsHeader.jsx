import { AppBar, Box, Button, IconButton, Toolbar } from "@mui/material";
import { ArrowLeft, GithubLogo, List as ListIcon, Translate } from "@phosphor-icons/react";
import { Brand } from "./Brand";

export function DocsHeader({
  onOpenNav,
  menuButtonRef,
  activeSection,
  navOpen = false,
  locale,
  onToggleLocale,
  copy
}) {
  return (
    <AppBar>
      <Toolbar sx={{ maxWidth: 1360, width: "100%", mx: "auto", px: { xs: 2, md: 3 }, gap: { xs: 1, md: 1.5 } }}>
        <IconButton
          ref={menuButtonRef}
          onClick={onOpenNav}
          aria-label={copy.menuAria}
          aria-expanded={navOpen}
          sx={{ display: { xs: "inline-flex", md: "none" }, ml: -0.5 }}
        >
          <ListIcon size={20} />
        </IconButton>

        <Brand href="../index.html" sublabel={copy.brandSub} compact={false} />

        <Button
          component="a"
          href="../index.html"
          variant="text"
          size="small"
          startIcon={<ArrowLeft size={15} />}
          sx={{ display: { xs: "none", sm: "inline-flex" }, color: "text.secondary", ml: 0.5 }}
        >
          {copy.home}
        </Button>

        <Box component="nav" aria-label={copy.navAria} sx={{ display: { xs: "none", lg: "flex" }, alignItems: "center", gap: 0.25, ml: 1 }}>
          {copy.sectionNav.map((item) => (
            <Button
              key={item.slug}
              component="a"
              href={`#/${item.slug}`}
              variant="text"
              size="small"
              aria-current={activeSection === item.slug ? "page" : undefined}
              sx={{
                color: activeSection === item.slug ? "text.primary" : "text.secondary",
                fontWeight: activeSection === item.slug ? 600 : 500,
                px: 1
              }}
            >
              {item.label}
            </Button>
          ))}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, ml: "auto", flex: "none" }}>
          <Button
            variant="outlined"
            size="small"
            onClick={onToggleLocale}
            aria-label={`${copy.switchLabel} — ${copy.switchAria}`}
            startIcon={<Translate size={14} />}
            sx={{ minWidth: 0, px: { xs: 0.9, sm: 1.25 }, whiteSpace: "nowrap" }}
          >
            {copy.switchLabel}
          </Button>
          <IconButton
            component="a"
            href="https://github.com/fluffyTighnari/levier"
            target="_blank"
            rel="noopener noreferrer"
            aria-label={copy.githubAria}
            sx={{ display: { xs: "none", sm: "inline-flex" } }}
          >
            <GithubLogo size={19} />
          </IconButton>
        </Box>
      </Toolbar>
    </AppBar>
  );
}
