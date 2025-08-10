stubgen main.py macro_processor.py out_types.py expr_ast.py
mv out/*.pyi ./
rm -rf out