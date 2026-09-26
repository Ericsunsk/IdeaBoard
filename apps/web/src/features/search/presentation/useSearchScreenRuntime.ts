import { defineAsyncComponent } from 'vue'
import { loadLibraryFloatingIsland } from '../../library'
import { useSearchScreenRuntime as useSearchScreenRuntimeImplementation } from '../application/useSearchScreenRuntime'

export const SearchLibraryFloatingIsland = defineAsyncComponent(loadLibraryFloatingIsland)

// Search screen orchestration lives in `../application/useSearchScreenRuntime`;
// this presentation entry stays thin so `SearchPage` and the dependency
// boundary fixtures keep resolving this path with zero call-site churn.
export const useSearchScreenRuntime = useSearchScreenRuntimeImplementation
