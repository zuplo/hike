// Bun supports importing text files via `with { type: "text" }`. TypeScript
// needs ambient declarations to know the shape.
declare module "*.sql" {
  const content: string;
  export default content;
}
declare module "*.md" {
  const content: string;
  export default content;
}
