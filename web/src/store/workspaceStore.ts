import { create } from 'zustand'
import type { RequestDefinition, ExecutionResult, TreeNode, EnvironmentDefinition } from '../types'

interface WorkspaceState {
  workspacePath: string | null
  activeEnv: string
  environments: EnvironmentDefinition[]
  tree: TreeNode[]
  activeRequest: RequestDefinition | null
  activeFilePath: string | null
  isExecuting: boolean
  lastResult: ExecutionResult | null
  activeTab: 'params' | 'headers' | 'auth' | 'body' | 'scripts' | 'settings'

  setWorkspacePath: (path: string | null) => void
  setActiveEnv: (env: string) => void
  setEnvironments: (envs: EnvironmentDefinition[]) => void
  setTree: (tree: TreeNode[]) => void
  setActiveRequest: (req: RequestDefinition | null, filePath?: string | null) => void
  setIsExecuting: (isExecuting: boolean) => void
  setLastResult: (result: ExecutionResult | null) => void
  setActiveTab: (tab: 'params' | 'headers' | 'auth' | 'body' | 'scripts' | 'settings') => void
  updateActiveRequest: (updater: (prev: RequestDefinition) => RequestDefinition) => void
}

const defaultRequest: RequestDefinition = {
  name: 'Untitled Request',
  method: 'GET',
  url: 'http://localhost:8080/api/health',
  headers: [
    { key: 'Accept', value: 'application/json', enabled: true },
  ],
  params: [],
  auth: { type: 'none' },
  body: { type: 'none' },
  scripts: {
    preRequest: '',
    postResponse: 'pb.test("Status code is 200", () => {\n  pb.expect(pb.response.status).to.eql(200);\n});',
  },
  settings: {
    followRedirects: true,
    verifySSL: true,
    timeoutMs: 30000,
  },
}

export const useWorkspaceStore = create<WorkspaceState>((set) => ({
  workspacePath: null,
  activeEnv: 'dev',
  environments: [
    {
      name: 'dev',
      variables: [
        { key: 'BASE_URL', value: 'http://localhost:8080', enabled: true },
      ],
    },
  ],
  tree: [],
  activeRequest: defaultRequest,
  activeFilePath: null,
  isExecuting: false,
  lastResult: null,
  activeTab: 'params',

  setWorkspacePath: (path) => set({ workspacePath: path }),
  setActiveEnv: (env) => set({ activeEnv: env }),
  setEnvironments: (environments) => set({ environments }),
  setTree: (tree) => set({ tree }),
  setActiveRequest: (activeRequest, activeFilePath = null) =>
    set({ activeRequest, activeFilePath, lastResult: null }),
  setIsExecuting: (isExecuting) => set({ isExecuting }),
  setLastResult: (lastResult) => set({ lastResult }),
  setActiveTab: (activeTab) => set({ activeTab }),
  updateActiveRequest: (updater) =>
    set((state) => ({
      activeRequest: state.activeRequest ? updater(state.activeRequest) : null,
    })),
}))
