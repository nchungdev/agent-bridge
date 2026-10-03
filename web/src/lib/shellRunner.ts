import { createContext } from "react";

/** Lets rendered code blocks run a command on the server (same path as typing "!cmd"). */
export const ShellRunnerContext = createContext<{ run: (command: string) => void } | null>(null);
