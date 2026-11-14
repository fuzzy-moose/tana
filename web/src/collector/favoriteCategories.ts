import type { FavoriteCategory } from './api'

export function categoryName(category: FavoriteCategory) { return category.name || `Category ${category.category}` }
