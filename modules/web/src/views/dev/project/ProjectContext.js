import { createContext, useContext } from 'react';

export const ProjectContext = createContext({
  owner: '',
  name: '',
  repo: null,
  isAdmin: false,
  reloadRepo: async () => {},
  ensureRepo: async () => null
});

export const useProjectContext = () => useContext(ProjectContext);
