import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import CssBaseline from "@mui/material/CssBaseline";
import { ThemeProvider } from "@mui/material/styles";
import { MotionConfig } from "motion/react";
import { theme } from "./theme";
import { LandingApp } from "./landing/LandingApp";

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <MotionConfig reducedMotion="user">
        <LandingApp />
      </MotionConfig>
    </ThemeProvider>
  </StrictMode>
);
