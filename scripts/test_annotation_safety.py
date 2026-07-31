#!/usr/bin/env python3
"""Prevent class members from breaking eagerly evaluated type annotations."""

import ast
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def has_future_annotations(tree: ast.Module) -> bool:
    return any(
        isinstance(node, ast.ImportFrom)
        and node.module == "__future__"
        and any(alias.name == "annotations" for alias in node.names)
        for node in tree.body
    )


def annotation_names(node: ast.AST) -> set[str]:
    annotations = []
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
        arguments = (
            *node.args.posonlyargs,
            *node.args.args,
            *node.args.kwonlyargs,
        )
        annotations.extend(
            argument.annotation for argument in arguments if argument.annotation
        )
        if node.args.vararg and node.args.vararg.annotation:
            annotations.append(node.args.vararg.annotation)
        if node.args.kwarg and node.args.kwarg.annotation:
            annotations.append(node.args.kwarg.annotation)
        if node.returns:
            annotations.append(node.returns)
    elif isinstance(node, ast.AnnAssign):
        annotations.append(node.annotation)
    return {
        item.id
        for annotation in annotations
        for item in ast.walk(annotation)
        if isinstance(item, ast.Name)
    }


def assigned_names(node: ast.AST) -> set[str]:
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
        return {node.name}
    if isinstance(node, ast.Assign):
        targets = node.targets
    elif isinstance(node, ast.AnnAssign):
        targets = [node.target]
    else:
        targets = []
    return {
        target.id
        for target in targets
        if isinstance(target, ast.Name)
    }


class AnnotationSafetyTests(unittest.TestCase):
    def test_class_members_do_not_shadow_eager_annotations(self):
        failures = []
        for path in sorted((ROOT / "bot").rglob("*.py")):
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
            if has_future_annotations(tree):
                continue
            for class_node in (
                node for node in ast.walk(tree) if isinstance(node, ast.ClassDef)
            ):
                bound_names = set()
                for node in class_node.body:
                    collisions = annotation_names(node) & bound_names
                    if collisions:
                        failures.append(
                            f"{path.relative_to(ROOT)}:{node.lineno} "
                            f"{class_node.name} shadows {sorted(collisions)}"
                        )
                    bound_names.update(assigned_names(node))
        self.assertEqual(failures, [], "\n" + "\n".join(failures))


if __name__ == "__main__":
    unittest.main()
