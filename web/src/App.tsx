import React from 'react'
import { TitleBar } from './components/layout/TitleBar'
import { CollectionTree } from './components/sidebar/CollectionTree'
import { RequestPanel } from './components/request/RequestPanel'
import { ResponsePanel } from './components/response/ResponsePanel'

export const App: React.FC = () => {
  return (
    <div className="flex flex-col h-screen w-screen overflow-hidden bg-zinc-950 text-zinc-100 select-none">
      {/* Frameless / Web Titlebar */}
      <TitleBar />

      {/* Main Workspace Layout */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Sidebar */}
        <CollectionTree />

        {/* Center Request Editor */}
        <div className="flex-1 flex overflow-hidden">
          <RequestPanel />

          {/* Right Response Viewer */}
          <ResponsePanel />
        </div>
      </div>
    </div>
  )
}

export default App
