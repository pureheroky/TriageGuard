import { create } from "zustand";

type UIStore = {
  mobileSidebarOpen: boolean;
  setMobileSidebarOpen: (open: boolean) => void;
};

export const useUIStore = create<UIStore>((set) => ({
  mobileSidebarOpen: false,
  setMobileSidebarOpen: (open) => set({ mobileSidebarOpen: open }),
}));
