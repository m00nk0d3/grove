#!/usr/bin/env node
import { add, list, remove } from "./commands.js";
import { load, save } from "./store.js";

const [command, ...args] = process.argv.slice(2);
const tags = args.flatMap((arg, i) => (args[i - 1] === "--tag" ? [arg] : []));

switch (command) {
  case "add":
    save(add(load(), args[0], tags, new Date()));
    break;
  case "list":
    list(load(), tags[0]).forEach((b, i) => console.log(`${i + 1}. ${b.title}  ${b.url}  [${b.tags.join(", ")}]`));
    break;
  case "remove":
    save(remove(load(), Number(args[0])));
    break;
  default:
    console.error("usage: bookmarks add <url> [--tag <tag>]... | list [--tag <tag>] | remove <number>");
    process.exit(2);
}
