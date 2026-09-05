export function getPageNumbers(adjacentCount: number, currentPage: number, totalPageCount: number): number[] {
  if (totalPageCount <= (adjacentCount + 1) * 2 + 3) {
    return Array.from({ length: totalPageCount }, (_, index) => index + 1)
  }

  const left: number[] = []
  const right: number[] = []

  let page = Math.max(2, currentPage - (adjacentCount + 1))
  while (page < currentPage && left.length < adjacentCount + 1) {
    left.push(page++)
  }

  page = currentPage + 1
  while (page < totalPageCount && right.length < adjacentCount + 1) {
    right.push(page++)
  }

  if (left.length === adjacentCount + 1) {
    let diff = (adjacentCount + 1) * 2 - (left.length + right.length)
    if (currentPage === totalPageCount) diff++

    page = left[0] - 1
    while (page > 1 && diff > 0) {
      left.push(page--)
      diff--
    }
    left.sort((a, b) => a - b)
  }

  if (right.length === adjacentCount + 1) {
    let diff = (adjacentCount + 1) * 2 - (left.length + right.length)
    if (currentPage === 1) diff++

    page = right[right.length - 1] + 1
    while (page < totalPageCount && diff > 0) {
      right.push(page++)
      diff--
    }
  }

  if (left.length >= 2 && left[0] !== 2 && left[1] !== 3) left[0] = -1
  if (right.length >= 2 && right[right.length - 1] !== totalPageCount - 1 && right[right.length - 2] !== totalPageCount - 2) {
    right[right.length - 1] = -1
  }

  const pages = [1]
  if (currentPage !== 1) pages.push(...left, currentPage)
  if (currentPage !== totalPageCount) pages.push(...right, totalPageCount)
  return pages
}
