#include <string>
#include <sstream>
#include <vector>
#include <memory>
#include <unordered_map>
#include <cstring>
#include <iostream>

// Debug logging macro
#define DEBUG_LOG(x) std::cerr << "[DEBUG] " << __func__ << ": " << x << std::endl

#include "clang/Tooling/Tooling.h"
#include "clang/AST/AST.h"
#include "clang/AST/ASTContext.h"
#include "clang/AST/Expr.h"
#include "clang/AST/RecursiveASTVisitor.h"
#include "clang/Frontend/FrontendAction.h"
#include "clang/Frontend/CompilerInstance.h"
#include "clang/Index/USRGeneration.h"
#include "clang/Lex/Lexer.h"

using namespace clang;
using namespace clang::tooling;

static std::unordered_map<std::string, std::string> customTypeNames;

static std::string exprToString(const Expr* e, ASTContext& ctx);

static std::string getUSRForType(QualType T, ASTContext &Ctx) {
    DEBUG_LOG("Getting USR for type: " << T.getAsString());
    llvm::SmallVector<char, 128> buf;
    if (!clang::index::generateUSRForType(T, Ctx, buf)) {
        std::string usr(buf.begin(), buf.end());
        DEBUG_LOG("Generated USR: " << usr);
        return usr;
    }
    DEBUG_LOG("Failed to generate USR");
    return {};
}

static std::string normalizeTypeName(QualType qt) {
    DEBUG_LOG("Normalizing type: " << qt.getAsString());
    QualType canonical = qt.getCanonicalType();
    DEBUG_LOG("Canonical type: " << canonical.getAsString());

    if (canonical->isIntegerType()) {
        DEBUG_LOG("Is integer type");
        if (canonical->isSpecificBuiltinType(BuiltinType::Int)) {
            DEBUG_LOG("Mapped to i32");
            return "i32";
        }
        if (canonical->isSpecificBuiltinType(BuiltinType::Short)) {
            DEBUG_LOG("Mapped to i16");
            return "i16";
        }
        if (canonical->isSpecificBuiltinType(BuiltinType::LongLong)) {
            DEBUG_LOG("Mapped to i64");
            return "i64";
        }
        if (canonical->isSpecificBuiltinType(BuiltinType::Char_U) ||
            canonical->isSpecificBuiltinType(BuiltinType::Char_S)) {
            DEBUG_LOG("Mapped to u8");
            return "u8";
        }
    }

    if (canonical->isFloatingType()) {
        DEBUG_LOG("Is floating type");
        if (canonical->isSpecificBuiltinType(BuiltinType::Float)) {
            DEBUG_LOG("Mapped to f32");
            return "f32";
        }
        if (canonical->isSpecificBuiltinType(BuiltinType::Double)) {
            DEBUG_LOG("Mapped to f64");
            return "f64";
        }
    }

    if (canonical->isPointerType()) {
        DEBUG_LOG("Is pointer type");
        QualType pointee = canonical->getPointeeType().getCanonicalType();
        DEBUG_LOG("Pointee type: " << pointee.getAsString());
        if (pointee->isSpecificBuiltinType(BuiltinType::Char_S) ||
            pointee->isSpecificBuiltinType(BuiltinType::Char_U)) {
            DEBUG_LOG("Mapped to str (C string)");
            return "str"; // special case for C strings
        }
        DEBUG_LOG("Mapped to anyptr");
        return "anyptr";
    }

    DEBUG_LOG("Using raw type string: " << qt.getAsString());
    return qt.getAsString();
}

static std::string resolveTypeName(QualType qt, ASTContext& ctx) {
    DEBUG_LOG("Resolving type name for: " << qt.getAsString());
    auto usr = getUSRForType(qt, ctx);
    DEBUG_LOG("Looking up USR in custom type names: " << usr);
    auto it = customTypeNames.find(usr);
    if (it != customTypeNames.end()) {
        DEBUG_LOG("Found custom type name: " << it->second);
        return it->second;
    }
    DEBUG_LOG("No custom type name found, normalizing");
    return normalizeTypeName(qt);
}


static std::string compoundLiteralToNamedInit(
    const CompoundLiteralExpr* cle, ASTContext& ctx)
{
    DEBUG_LOG("Processing compound literal expression");
    QualType qt = cle->getType();
    DEBUG_LOG("Type: " << qt.getAsString());
    
    const RecordType* rt = qt->getAs<RecordType>();
    if (!rt) {
        DEBUG_LOG("Not a record type");
        return "<not a record>";
    }

    const RecordDecl* rd = rt->getDecl();
    DEBUG_LOG("Record name: " << rd->getNameAsString());
    
    const InitListExpr* ile = dyn_cast<InitListExpr>(cle->getInitializer());
    if (!ile) {
        DEBUG_LOG("Not an init list expression");
        return "<not an init list>";
    }
    DEBUG_LOG("Found init list with " << ile->getNumInits() << " initializers");

    std::ostringstream out;
    std::string type_name = resolveTypeName(qt, ctx);
    DEBUG_LOG("Using type name: " << type_name);
    out << type_name << "{";

    unsigned i = 0;
    for (auto field : rd->fields()) {
        if (i > 0) out << ", ";
        std::string field_name = field->getName().str();
        DEBUG_LOG("Processing field " << i << ": " << field_name);
        out << field_name << "="; // No leading dot
        
        const Expr* initExpr = ile->getInit(i)->IgnoreImpCasts();
        if (auto nestedCle = dyn_cast<CompoundLiteralExpr>(initExpr)) {
            DEBUG_LOG("Found nested compound literal for field " << field_name);
            out << compoundLiteralToNamedInit(nestedCle, ctx);
        } else {
            DEBUG_LOG("Converting expression to string for field " << field_name);
            out << exprToString(initExpr, ctx);
        }
        i++;
    }
    out << "}";
    std::string result = out.str();
    DEBUG_LOG("Generated initializer: " << result);
    return result;
}

static std::string exprToString(const Expr* e, ASTContext& ctx) {
    DEBUG_LOG("Converting expression to string, kind: " << e->getStmtClassName());

    if (auto intLit = dyn_cast<IntegerLiteral>(e)) {
        std::string value = std::to_string(intLit->getValue().getSExtValue());
        DEBUG_LOG("Integer literal value: " << value);
        return value;
    }

    if (auto floatLit = dyn_cast<FloatingLiteral>(e)) {
        SmallString<16> str;
        floatLit->getValue().toString(str);
        DEBUG_LOG("Float literal value: " << str.c_str());
        return str.c_str();
    }

    if (auto strLit = dyn_cast<StringLiteral>(e)) {
        std::string value = "\"" + strLit->getString().str() + "\".ref()";
        DEBUG_LOG("String literal value: " << value);
        return value;
    }

    if (auto cle = dyn_cast<CompoundLiteralExpr>(e)) {
        DEBUG_LOG("Found nested compound literal");
        return compoundLiteralToNamedInit(cle, ctx);
    }

    // fallback: dump the source text
    DEBUG_LOG("Using fallback: source text extraction");
    SourceManager& sm = ctx.getSourceManager();
    SourceLocation b(e->getBeginLoc()), _e(e->getEndLoc());
    CharSourceRange range = CharSourceRange::getTokenRange(b, _e);
    std::string text = Lexer::getSourceText(range, sm, ctx.getLangOpts()).str();
    DEBUG_LOG("Extracted source text: " << text);
    return text;
}

extern "C" {

// Set the custom name map from Python
void set_custom_type_names(const char* const* usrs, const char* const* names, int count) {
    DEBUG_LOG("Setting custom type names, count: " << count);
    customTypeNames.clear();
    for (int i = 0; i < count; ++i) {
        DEBUG_LOG("Entry " << i << ": USR='" << usrs[i] << "' -> Name='" << names[i] << "'");
        customTypeNames[usrs[i]] = names[i];
    }
    DEBUG_LOG("Custom type names map now has " << customTypeNames.size() << " entries");
}

const char* macro_to_named_initializer(
    const char* header_path,
    const char* define_name,
    const char* const* clang_args,
    int num_args
) {
    DEBUG_LOG("Processing macro: " << define_name << " from header: " << header_path);
    
    // Skip if header was included with <>
    if (header_path[0] == '<' && header_path[std::string(header_path).length()-1] == '>') {
        DEBUG_LOG("Skipping system header: " << header_path);
        return nullptr;
    }

    std::string code;
    code += "#include \"" + std::string(header_path) + "\"\n";
    code += "const auto __dummy_var = " + std::string(define_name) + ";\n";
    DEBUG_LOG("Generated code:\n" << code);

    std::vector<std::string> args = {"-x", "c", "-std=c11"};
    // Add include path for the header's directory
    std::string header_dir = header_path;
    size_t last_slash = header_dir.find_last_of("/\\");
    if (last_slash != std::string::npos) {
        header_dir = header_dir.substr(0, last_slash);
        args.push_back("-I" + header_dir);
    }
    
    for (int i = 0; i < num_args; i++) {
        DEBUG_LOG("Clang arg " << i << ": " << clang_args[i]);
        args.push_back(clang_args[i]);
    }

    DEBUG_LOG("Building AST from code");
    auto unit = buildASTFromCodeWithArgs(code, args, "macro_eval.c");
    if (!unit) {
        DEBUG_LOG("Failed to build AST");
        return nullptr;
    }
    DEBUG_LOG("Successfully built AST");

    ASTContext &ctx = unit->getASTContext();
    TranslationUnitDecl *tu = ctx.getTranslationUnitDecl();

    DEBUG_LOG("Searching for __dummy_var in declarations");
    for (Decl *d : tu->decls()) {
        if (auto *vd = dyn_cast<VarDecl>(d)) {
            if (vd->getName() == "__dummy_var") {
                DEBUG_LOG("Found __dummy_var");
                const Expr* init = vd->getInit()->IgnoreImpCasts();
                if (auto cle = dyn_cast<CompoundLiteralExpr>(init)) {
                    DEBUG_LOG("Found compound literal expression");
                    std::string type_name = resolveTypeName(cle->getType(), ctx);
                    DEBUG_LOG("Resolved type name: " << type_name);
                    std::string init_str = compoundLiteralToNamedInit(cle, ctx);
                    DEBUG_LOG("Generated initializer: " << init_str);
                    
                    std::string result = type_name + " " + define_name + " = " + init_str + ";";
                    DEBUG_LOG("Final result: " << result);
                    
                    char *cstr = (char*)malloc(result.size() + 1);
                    strcpy(cstr, result.c_str());
                    return cstr;
                } else {
                    DEBUG_LOG("Init expression is not a compound literal");
                }
            }
        }
    }

    DEBUG_LOG("No suitable declaration found");
    return nullptr;
}

void free_cstr(const char* s) {
    if (s) free((void*)s);
}

}
