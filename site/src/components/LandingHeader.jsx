import { useRef, useState } from "react";
import {
  AppBar,
  Box,
  Button,
  IconButton,
  List,
  ListItemButton,
  Toolbar,
  Typography
} from "@mui/material";
import { GithubLogo, List as ListIcon } from "@phosphor-icons/react";
import { Brand } from "./Brand";
import { MobileDrawer } from "./MobileDrawer";

const NAV = [
  { id: "runtime", label: "Runtime" },
  { id: "surface", label: "One run" },
  { id: "integration", label: "CLI control plane" },
  { id: "scope", label: "Boundaries" }
];

export function LandingHeader({ activeSection }) {
  const [open, setOpen] = useState(false);
  const menuButtonRef = useRef(null);

  const handleClose = (event, reason) => {
    setOpen(false);
    if (["backdropClick", "escapeKeyDown", "closeButton"].includes(reason)) {
      menuButtonRef.current?.focus();
    }
  };

  return (
    <>
      <AppBar>
        <Toolbar sx={{ maxWidth: 1200, width: "100%", mx: "auto", px: { xs: 2, md: 3 }, gap: { xs: 1, md: 2 } }}>
          <IconButton
            ref={menuButtonRef}
            onClick={() => setOpen(true)}
            aria-label="Open navigation"
            aria-expanded={open}
            sx={{ display: { xs: "inline-flex", md: "none" }, ml: -0.5 }}
          >
            <ListIcon size={20} />
          </IconButton>

          <Brand href="#top" sublabel="Agent Run Control Plane" />

          <Box component="nav" aria-label="Primary" sx={{ display: { xs: "none", md: "flex" }, alignItems: "center", gap: 0.5, ml: 1.5 }}>
            {NAV.map((item) => {
              const active = activeSection === item.id;
              return (
                <Button
                  key={item.id}
                  component="a"
                  href={`#${item.id}`}
                  variant="text"
                  size="small"
                  aria-current={active ? "location" : undefined}
                  sx={{
                    color: active ? "text.primary" : "text.secondary",
                    fontWeight: active ? 600 : 500,
                    gap: 0.75
                  }}
                >
                  {item.label}
                  {active ? <Box component="span" sx={{ width: 4, height: 4, borderRadius: "50%", bgcolor: "primary.main" }} /> : null}
                </Button>
              );
            })}
          </Box>

          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, ml: "auto", flex: "none" }}>
            <IconButton
              component="a"
              href="https://github.com/fluffyTighnari/levier"
              target="_blank"
              rel="noopener noreferrer"
              aria-label="Levier on GitHub (opens in a new tab)"
              sx={{ display: { xs: "none", sm: "inline-flex" } }}
            >
              <GithubLogo size={19} />
            </IconButton>
            <Button component="a" href="./docs/" variant="outlined" size="small" sx={{ whiteSpace: "nowrap" }}>
              Read the docs
            </Button>
          </Box>
        </Toolbar>
      </AppBar>

      <MobileDrawer
        open={open}
        onClose={handleClose}
        title="Navigation"
        footer={
          <Box sx={{ display: "grid", gap: 1 }}>
            <Typography variant="caption" sx={{ color: "text.secondary" }}>
              Runtime, designed by you.
            </Typography>
            <Button component="a" href="./docs/" variant="contained" size="small" onClick={() => setOpen(false)}>
              Read the docs
            </Button>
          </Box>
        }
      >
        <List disablePadding sx={{ mt: 2 }}>
          {NAV.map((item) => (
            <ListItemButton
              key={item.id}
              component="a"
              href={`#${item.id}`}
              onClick={() => setOpen(false)}
              selected={activeSection === item.id}
              sx={{ mb: 0.5 }}
            >
              {item.label}
            </ListItemButton>
          ))}
          <ListItemButton
            component="a"
            href="https://github.com/fluffyTighnari/levier"
            target="_blank"
            rel="noopener noreferrer"
            onClick={() => setOpen(false)}
            sx={{ mt: 1 }}
          >
            GitHub
          </ListItemButton>
        </List>
      </MobileDrawer>
    </>
  );
}
